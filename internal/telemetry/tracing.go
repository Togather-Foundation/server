package telemetry

import (
	"context"
	"fmt"

	"github.com/Togather-Foundation/server/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// InitTracing initializes OpenTelemetry tracing based on configuration.
// Returns a shutdown function that should be called on application exit, and any error encountered.
//
// Configuration options:
//   - Enabled: must be true to enable tracing (default: false)
//   - Exporter: "stdout" (console output), "otlp" (OpenTelemetry Collector), or "none"
//   - ServiceName: identifies this service in traces
//   - OTLPEndpoint: gRPC endpoint for OTLP exporter (only used when Exporter is "otlp")
//   - SampleRate: percentage of traces to sample (0.0 to 1.0)
//
// Usage:
//
//	shutdown, err := telemetry.InitTracing(ctx, cfg.Tracing, version)
//	if err != nil {
//	    logger.Error().Err(err).Msg("failed to initialize tracing")
//	}
//	defer shutdown(ctx)
func InitTracing(ctx context.Context, cfg config.TracingConfig, serviceVersion string) (func(context.Context) error, error) {
	// If tracing is disabled, return a no-op shutdown function
	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	// Validate sample rate
	if cfg.SampleRate < 0.0 || cfg.SampleRate > 1.0 {
		return nil, fmt.Errorf("invalid sample rate %f: must be between 0.0 and 1.0", cfg.SampleRate)
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create exporter based on configuration
	var exporter sdktrace.SpanExporter
	switch cfg.Exporter {
	case "stdout":
		exporter, err = stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout exporter: %w", err)
		}
	case "otlp":
		exporter, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlptracegrpc.WithInsecure(), // Use TLS in production
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
		}
	case "none":
		// No-op exporter (traces are generated but not exported)
		exporter = &noopExporter{}
	default:
		return nil, fmt.Errorf("unsupported exporter: %s (must be 'stdout', 'otlp', or 'none')", cfg.Exporter)
	}

	// Create sampler based on sample rate
	var sampler sdktrace.Sampler
	if cfg.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SampleRate <= 0.0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	// Create trace provider with batch span processor for better performance
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
		sdktrace.WithBatcher(exporter),
	)

	// Register as global trace provider
	otel.SetTracerProvider(tp)

	// Set global propagator to extract and inject trace context
	// W3C Trace Context is the standard for distributed tracing
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Return shutdown function that properly flushes pending spans
	shutdown := func(ctx context.Context) error {
		return tp.Shutdown(ctx)
	}

	return shutdown, nil
}

// GetTracer returns a tracer for the given name.
// Use this to create spans in your application code.
//
// Example:
//
//	tracer := telemetry.GetTracer("github.com/Togather-Foundation/server/internal/api")
//	ctx, span := tracer.Start(r.Context(), "HandleEventCreate")
//	defer span.End()
func GetTracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// noopExporter is a no-op span exporter for testing
type noopExporter struct{}

func (e *noopExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e *noopExporter) Shutdown(ctx context.Context) error {
	return nil
}
