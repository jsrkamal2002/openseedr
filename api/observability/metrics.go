package observability

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Meter is the application-wide OTel meter.
var Meter metric.Meter

// Instruments holds all custom metric instruments for openseedr.
type Instruments struct {
	// HTTP
	HTTPRequestsTotal   metric.Int64Counter
	HTTPRequestDuration metric.Float64Histogram
	HTTPActiveRequests  metric.Int64UpDownCounter

	// Torrents
	TorrentsAdded     metric.Int64Counter
	TorrentsDeleted   metric.Int64Counter
	TorrentsActive    metric.Int64ObservableGauge
	TorrentBytesTotal metric.Int64Counter

	// Storage
	StorageUsedBytes metric.Int64ObservableGauge

	// Auth
	LoginAttempts  metric.Int64Counter
	LoginFailures  metric.Int64Counter
	OAuthCallbacks metric.Int64Counter
}

// App holds the global Instruments instance.
var App *Instruments

// InitMetrics creates all metric instruments. Must be called after Init().
func InitMetrics() error {
	Meter = otel.Meter(serviceName)

	var err error
	i := &Instruments{}

	// ── HTTP ─────────────────────────────────────────────────────────────────
	i.HTTPRequestsTotal, err = Meter.Int64Counter(
		"http.server.requests.total",
		metric.WithDescription("Total number of HTTP requests received"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return err
	}

	i.HTTPRequestDuration, err = Meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("HTTP request latency in milliseconds"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000),
	)
	if err != nil {
		return err
	}

	i.HTTPActiveRequests, err = Meter.Int64UpDownCounter(
		"http.server.active_requests",
		metric.WithDescription("Number of HTTP requests currently being processed"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return err
	}

	// ── Torrents ─────────────────────────────────────────────────────────────
	i.TorrentsAdded, err = Meter.Int64Counter(
		"openseedr.torrents.added.total",
		metric.WithDescription("Total torrents added by all users"),
		metric.WithUnit("{torrent}"),
	)
	if err != nil {
		return err
	}

	i.TorrentsDeleted, err = Meter.Int64Counter(
		"openseedr.torrents.deleted.total",
		metric.WithDescription("Total torrents deleted by all users"),
		metric.WithUnit("{torrent}"),
	)
	if err != nil {
		return err
	}

	i.TorrentsActive, err = Meter.Int64ObservableGauge(
		"openseedr.torrents.active",
		metric.WithDescription("Number of currently active (downloading/seeding) torrents"),
		metric.WithUnit("{torrent}"),
	)
	if err != nil {
		return err
	}

	i.TorrentBytesTotal, err = Meter.Int64Counter(
		"openseedr.torrents.downloaded.bytes",
		metric.WithDescription("Total bytes downloaded across all torrents"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return err
	}

	// ── Storage ───────────────────────────────────────────────────────────────
	i.StorageUsedBytes, err = Meter.Int64ObservableGauge(
		"openseedr.storage.used.bytes",
		metric.WithDescription("Disk bytes currently used per user"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return err
	}

	// ── Auth ──────────────────────────────────────────────────────────────────
	i.LoginAttempts, err = Meter.Int64Counter(
		"openseedr.auth.login.attempts.total",
		metric.WithDescription("Total login attempts (local + oauth)"),
		metric.WithUnit("{attempt}"),
	)
	if err != nil {
		return err
	}

	i.LoginFailures, err = Meter.Int64Counter(
		"openseedr.auth.login.failures.total",
		metric.WithDescription("Total failed login attempts"),
		metric.WithUnit("{attempt}"),
	)
	if err != nil {
		return err
	}

	i.OAuthCallbacks, err = Meter.Int64Counter(
		"openseedr.auth.oauth.callbacks.total",
		metric.WithDescription("Total OAuth callback completions"),
		metric.WithUnit("{callback}"),
	)
	if err != nil {
		return err
	}

	App = i
	return nil
}

// RecordHTTPRequest is a helper to record HTTP metrics uniformly.
func RecordHTTPRequest(ctx context.Context, method, route, statusClass string, durationMs float64) {
	if App == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String("http.method", method),
		attribute.String("http.route", route),
		attribute.String("http.status_class", statusClass),
	)
	App.HTTPRequestsTotal.Add(ctx, 1, attrs)
	App.HTTPRequestDuration.Record(ctx, durationMs, attrs)
}

// RecordTorrentAdded records a torrent-add event with user context.
func RecordTorrentAdded(ctx context.Context, userID string) {
	if App == nil {
		return
	}
	App.TorrentsAdded.Add(ctx, 1,
		metric.WithAttributes(attribute.String("user.id", userID)),
	)
}

// RecordTorrentDeleted records a torrent-delete event with user context.
func RecordTorrentDeleted(ctx context.Context, userID string) {
	if App == nil {
		return
	}
	App.TorrentsDeleted.Add(ctx, 1,
		metric.WithAttributes(attribute.String("user.id", userID)),
	)
}

// RecordLoginAttempt records an authentication attempt.
func RecordLoginAttempt(ctx context.Context, provider string, success bool) {
	if App == nil {
		return
	}
	App.LoginAttempts.Add(ctx, 1,
		metric.WithAttributes(attribute.String("auth.provider", provider)),
	)
	if !success {
		App.LoginFailures.Add(ctx, 1,
			metric.WithAttributes(attribute.String("auth.provider", provider)),
		)
	}
}

// SpanEvent adds a structured event to the current span. Useful for logging
// within traced operations without allocating a child span.
func SpanEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(ctx).AddEvent(name, trace.WithAttributes(attrs...))
}

// Log emits a structured log line. The current trace/span IDs are automatically
// injected by the otelslog bridge.
func Log(ctx context.Context, level slog.Level, msg string, args ...any) {
	slog.Default().Log(ctx, level, msg, args...)
}

// Elapsed is a convenience helper: call it with time.Now() at the start of an
// operation and it returns the milliseconds elapsed.
func Elapsed(start time.Time) float64 {
	return float64(time.Since(start).Milliseconds())
}
