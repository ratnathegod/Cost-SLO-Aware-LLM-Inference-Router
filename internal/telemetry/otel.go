package telemetry

import (
    "context"
    "strings"
    "time"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/propagation"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// InitOTEL configures OTEL tracing (and sets global propagator)
func InitOTEL(ctx context.Context, serviceName, otlpEndpoint string) (func(context.Context) error, error) {
    if otlpEndpoint == "" {
        // No exporter configured; set noop provider
        otel.SetTracerProvider(sdktrace.NewTracerProvider())
        otel.SetTextMapPropagator(propagation.TraceContext{})
        return func(context.Context) error { return nil }, nil
    }

    // Create OTLP trace exporter (gRPC)
    opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(otlpEndpoint)}
    // If endpoint doesn't look like host:port without TLS, enable insecure
    if !strings.HasSuffix(otlpEndpoint, ":4317") && !strings.Contains(otlpEndpoint, ":") {
        // allow something like localhost without port; let client handle
    }
    opts = append(opts, otlptracegrpc.WithInsecure())

    exp, err := otlptracegrpc.New(ctx, opts...)
    if err != nil {
        return nil, err
    }

    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exp,
            sdktrace.WithMaxExportBatchSize(512),
            sdktrace.WithBatchTimeout(3*time.Second),
        ),
        sdktrace.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceName(serviceName),
        )),
    )
    otel.SetTracerProvider(tp)
    otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

    shutdown := func(ctx context.Context) error {
        return tp.Shutdown(ctx)
    }
    return shutdown, nil
}
