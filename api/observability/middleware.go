package observability

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// OtelMiddleware returns the otelgin auto-instrumentation middleware.
// It creates a span per request, sets standard HTTP attributes, and
// propagates the incoming W3C trace-context headers.
func OtelMiddleware() gin.HandlerFunc {
	return otelgin.Middleware(serviceName)
}

// MetricsMiddleware records per-request HTTP metrics and emits a structured
// access log line with trace_id + span_id so logs are correlated with traces.
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		if App != nil {
			App.HTTPActiveRequests.Add(c.Request.Context(), 1,
				metric.WithAttributes(attribute.String("http.method", c.Request.Method), attribute.String("http.route", c.FullPath())))
		}

		c.Next()

		durationMs := float64(time.Since(start).Milliseconds())
		statusCode := c.Writer.Status()
		statusClass := fmt.Sprintf("%dxx", statusCode/100)
		route := c.FullPath()
		if route == "" {
			route = "unknown"
		}

		ctx := c.Request.Context()

		if App != nil {
			App.HTTPActiveRequests.Add(ctx, -1,
				metric.WithAttributes(attribute.String("http.method", c.Request.Method), attribute.String("http.route", route)))
			RecordHTTPRequest(ctx, c.Request.Method, route, statusClass, durationMs)
		}

		// Enrich the span with final status
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(
			attribute.Int("http.status_code", statusCode),
			attribute.Float64("http.duration_ms", durationMs),
		)
		if statusCode >= 500 {
			span.SetStatus(codes.Error, http.StatusText(statusCode))
		}

		// Structured access log — trace_id & span_id are appended so log
		// aggregators (Loki, etc.) can correlate with Jaeger traces.
		spanCtx := span.SpanContext()
		slog.InfoContext(ctx, "http.request",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.String("route", route),
			slog.Int("status", statusCode),
			slog.Float64("duration_ms", durationMs),
			slog.String("client_ip", c.ClientIP()),
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		)
	}
}

// RecoveryMiddleware catches panics, records them as span errors, emits a
// structured error log, and returns HTTP 500.
func RecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				ctx := c.Request.Context()
				span := trace.SpanFromContext(ctx)
				span.RecordError(fmt.Errorf("%v", r))
				span.SetStatus(codes.Error, "panic recovered")

				slog.ErrorContext(ctx, "panic recovered",
					slog.Any("panic", r),
					slog.String("path", c.Request.URL.Path),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "internal server error",
				})
			}
		}()
		c.Next()
	}
}

// TraceID returns the trace ID from the current request context as a string.
// Useful for injecting into API responses so clients can correlate errors.
func TraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// StatusLabel converts an integer HTTP status code to its text label,
// e.g. 404 → "Not Found".
func StatusLabel(code int) string {
	return strconv.Itoa(code) + " " + http.StatusText(code)
}
