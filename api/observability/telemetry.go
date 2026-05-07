// Package observability wires up OpenTelemetry tracing, metrics, and logging
// for the openseedr API. It exports via OTLP/gRPC to the OTel Collector and
// also exposes a Prometheus /metrics scrape endpoint.
package observability

import (
	"context"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const serviceName = "openseedr-api"

// Providers holds references to all OTel SDK providers so they can be
// shut down gracefully on application exit.
type Providers struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	LoggerProvider *sdklog.LoggerProvider
}

// Logger is the application-wide structured logger. It is backed by the OTel
// log bridge so every log record is forwarded to the OTel Collector alongside
// its trace context (trace_id / span_id are automatically injected).
var Logger *slog.Logger

// Init sets up all OTel providers and returns a Providers struct.
// Call Shutdown on it during graceful teardown.
func Init(ctx context.Context) (*Providers, error) {
	res, err := newResource(ctx)
	if err != nil {
		return nil, err
	}

	collectorEndpoint := getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317")

	conn, err := grpc.NewClient(collectorEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}

	// ── Tracing ──────────────────────────────────────────────────────────────
	tp, err := newTracerProvider(ctx, conn, res)
	if err != nil {
		return nil, err
	}
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// ── Metrics ──────────────────────────────────────────────────────────────
	mp, err := newMeterProvider(ctx, conn, res)
	if err != nil {
		return nil, err
	}
	otel.SetMeterProvider(mp)

	// ── Logging ──────────────────────────────────────────────────────────────
	lp, err := newLoggerProvider(ctx, conn, res)
	if err != nil {
		return nil, err
	}
	global.SetLoggerProvider(lp)

	// Wire slog → OTel log bridge so structured logs carry trace context
	Logger = slog.New(otelslog.NewHandler(serviceName))
	slog.SetDefault(Logger)

	return &Providers{
		TracerProvider: tp,
		MeterProvider:  mp,
		LoggerProvider: lp,
	}, nil
}

// Shutdown flushes and stops all OTel providers. Should be deferred in main().
func (p *Providers) Shutdown(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := p.TracerProvider.Shutdown(ctx); err != nil {
		slog.Error("tracer provider shutdown error", "error", err)
	}
	if err := p.MeterProvider.Shutdown(ctx); err != nil {
		slog.Error("meter provider shutdown error", "error", err)
	}
	if err := p.LoggerProvider.Shutdown(ctx); err != nil {
		slog.Error("logger provider shutdown error", "error", err)
	}
}

// ── internal helpers ─────────────────────────────────────────────────────────

func newResource(ctx context.Context) (*sdkresource.Resource, error) {
	return sdkresource.New(ctx,
		sdkresource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(getEnv("APP_VERSION", "0.1.0")),
			semconv.DeploymentEnvironment(getEnv("APP_ENV", "development")),
		),
		sdkresource.WithOS(),
		// WithProcess() includes WithProcessOwner() which calls user.Current().
		// This fails in CGO-disabled binaries running on scratch images without
		// $USER set. Use the individual detectors that don't require user info.
		sdkresource.WithProcessPID(),
		sdkresource.WithProcessExecutableName(),
		sdkresource.WithProcessExecutablePath(),
		sdkresource.WithProcessRuntimeName(),
		sdkresource.WithProcessRuntimeVersion(),
		sdkresource.WithProcessRuntimeDescription(),
		sdkresource.WithHost(),
	)
}

func newTracerProvider(ctx context.Context, conn *grpc.ClientConn, res *sdkresource.Resource) (*sdktrace.TracerProvider, error) {
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exp,
			sdktrace.WithBatchTimeout(5*time.Second),
		),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	return tp, nil
}

func newMeterProvider(ctx context.Context, conn *grpc.ClientConn, res *sdkresource.Resource) (*sdkmetric.MeterProvider, error) {
	// OTLP exporter → OTel Collector → Prometheus remote-write
	otlpExp, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, err
	}

	// Prometheus pull exporter — served on /metrics by the API itself
	promExp, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(otlpExp,
				sdkmetric.WithInterval(15*time.Second),
			),
		),
		sdkmetric.WithReader(promExp),
	)
	return mp, nil
}

func newLoggerProvider(ctx context.Context, conn *grpc.ClientConn, res *sdkresource.Resource) (*sdklog.LoggerProvider, error) {
	exp, err := otlploggrpc.New(ctx, otlploggrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, err
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(
			sdklog.NewBatchProcessor(exp,
				sdklog.WithExportInterval(5*time.Second),
			),
		),
	)
	return lp, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
