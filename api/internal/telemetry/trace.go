package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	otlptracehttp "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
)

var tracer = otel.Tracer("fractal-api")

// InitTracer sets up an OTLP trace exporter (Elastic) and returns a shutdowndable provider.
func InitTracer(endpoint string) (*sdktrace.TracerProvider, error) {
	exp, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	res, _ := resource.New(context.Background(),
		resource.WithAttributes(
			attribute.String("service.name", "fractal-api"),
			attribute.String("service.version", "1.0.0"),
		),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}

// StartRequestSpan begins a span for an HTTP request and attaches request ID
// and case ID (if present) as attributes.
func StartRequestSpan(ctx context.Context, method, path string, reqID string, caseID string) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String("http.method", method),
		attribute.String("http.path", path),
		attribute.String("request_id", reqID),
	}
	if caseID != "" {
		attrs = append(attrs, attribute.String("case_id", caseID))
	}
	ctx, span := tracer.Start(ctx, "http.request", trace.WithAttributes(attrs...))
	return ctx, span
}

// EndSpan marks the span status and ends it.
func EndSpan(span trace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
