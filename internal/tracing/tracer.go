// Package tracing initializes the OpenTelemetry tracing pipeline for every
// Tiny microservice. It configures an OTLP/HTTP exporter that ships spans
// to Jaeger (or any OTLP-compatible backend), attaches service metadata
// via semantic conventions, and sets a configurable sampling strategy.
//
// Each service calls InitTracer once at startup and defers ShutdownTracer
// to flush pending spans before exiting. The returned TracerProvider is
// also registered as the global OTel provider so that instrumentation
// libraries (e.g., otelgrpc, otelhttp) pick it up automatically.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config holds the parameters needed to set up the tracing exporter.
// When Enabled is false, a no-op TracerProvider is installed so that
// tracing instrumentation compiles in but produces zero overhead.
type Config struct {
	Enabled        bool
	JaegerEndpoint string // Full URL of the OTLP/HTTP collector (e.g., http://jaeger:4318/v1/traces).
	ServiceName    string
	ServiceVersion string
	// SampleRate controls trace sampling: 1.0 = always, 0.0 = never,
	// values in between use probabilistic TraceIDRatioBased sampling.
	SampleRate float64
}

// InitTracer creates and globally registers an OpenTelemetry TracerProvider.
//
// When tracing is disabled (cfg.Enabled == false), it installs a bare
// TracerProvider with no exporter so that span creation calls throughout the
// code become no-ops without requiring nil checks everywhere.
//
// When enabled, the function:
//  1. Creates an OTLP/HTTP exporter pointed at cfg.JaegerEndpoint.
//  2. Builds an OTel resource tagged with the service name and version
//     (following OTel semantic conventions) so Jaeger can group traces.
//  3. Selects a sampler based on cfg.SampleRate -- AlwaysSample for 1.0,
//     NeverSample for 0.0, and TraceIDRatioBased for fractional rates.
//  4. Sets the global TextMapPropagator to W3C TraceContext + Baggage so
//     that trace context propagates across HTTP and gRPC boundaries.
func InitTracer(cfg Config) (*sdktrace.TracerProvider, error) {
	if !cfg.Enabled {
		tp := sdktrace.NewTracerProvider()
		otel.SetTracerProvider(tp)
		return tp, nil
	}

	exporter, err := otlptracehttp.New(
		context.Background(),
		otlptracehttp.WithEndpointURL(cfg.JaegerEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Choose sampling strategy based on the configured rate. Using the
	// boundary values (>= 1.0, <= 0) avoids floating-point edge cases
	// where TraceIDRatioBased might behave unexpectedly at the extremes.
	var sampler sdktrace.Sampler
	if cfg.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SampleRate <= 0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Register globally so that otelgrpc/otelhttp interceptors and any
	// manual otel.Tracer("name") calls use this provider automatically.
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

// ShutdownTracer flushes any pending spans and releases exporter resources.
// Pass a context with a deadline to bound the flush time; in production a
// 5-second timeout is typical.
func ShutdownTracer(ctx context.Context, tp *sdktrace.TracerProvider) error {
	return tp.Shutdown(ctx)
}
