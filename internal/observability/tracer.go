// Package observability sets up OpenTelemetry tracing and exports spans to
// Langfuse (https://langfuse.com) via its OTLP/HTTP ingestion endpoint.
//
// When LANGFUSE_PUBLIC_KEY and LANGFUSE_SECRET_KEY are unset the package
// installs a no-op tracer provider so tracer.Start() calls in callers remain
// safe but no spans are exported.
package observability

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TracerName is the instrumentation library name used for spans produced
	// by this application.
	TracerName = "github.com/tomr1233/intake-form-api"

	// Langfuse attribute keys. See:
	// https://langfuse.com/integrations/native/opentelemetry
	AttrObservationType    = "langfuse.observation.type"
	AttrObservationInput   = "langfuse.observation.input"
	AttrObservationOutput  = "langfuse.observation.output"
	AttrSessionID          = "langfuse.session.id"
	AttrUserID             = "langfuse.user.id"
	AttrTraceTags          = "langfuse.trace.tags"
	AttrTraceMetadataPfx   = "langfuse.trace.metadata."
	AttrObservationMetaPfx = "langfuse.observation.metadata."

	ObservationGeneration = "generation"
	ObservationSpan       = "span"
	ObservationEvent      = "event"

	// Default to Langfuse Cloud US since the Gemini API hint and the rest of
	// the stack run there; override with LANGFUSE_HOST.
	defaultLangfuseHost = "https://us.cloud.langfuse.com"
)

// ShutdownFunc flushes any buffered spans and tears down the exporter. Call
// before the process exits or batched spans will be lost.
type ShutdownFunc func(context.Context) error

// Setup installs a global TracerProvider that exports to Langfuse over OTLP
// HTTP. It returns a shutdown function that the caller MUST invoke before
// exit (the OTLP exporter is batched).
//
// If LANGFUSE_PUBLIC_KEY or LANGFUSE_SECRET_KEY is missing, the function logs
// nothing here (the caller decides what to log) and installs the no-op
// TracerProvider implied by go.opentelemetry.io/otel's default. In that case
// the returned shutdown is a no-op.
func Setup(ctx context.Context, serviceName, serviceVersion string) (ShutdownFunc, bool, error) {
	publicKey := os.Getenv("LANGFUSE_PUBLIC_KEY")
	secretKey := os.Getenv("LANGFUSE_SECRET_KEY")
	if publicKey == "" || secretKey == "" {
		return func(context.Context) error { return nil }, false, nil
	}

	// Accept the same env var names Langfuse's other SDKs use, in priority
	// order. Picking up LANGFUSE_BASE_URL / LANGFUSE_BASEURL avoids silent
	// region mismatches when the .env was copied from Langfuse docs.
	host := firstNonEmpty(
		os.Getenv("LANGFUSE_HOST"),
		os.Getenv("LANGFUSE_BASE_URL"),
		os.Getenv("LANGFUSE_BASEURL"),
	)
	host = strings.TrimRight(host, "/")
	if host == "" {
		host = defaultLangfuseHost
	}
	endpoint, err := url.Parse(host)
	if err != nil {
		return nil, false, fmt.Errorf("invalid LANGFUSE_HOST %q: %w", host, err)
	}

	auth := base64.StdEncoding.EncodeToString([]byte(publicKey + ":" + secretKey))
	httpOpts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpoint.Host),
		otlptracehttp.WithURLPath("/api/public/otel/v1/traces"),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization":                "Basic " + auth,
			"x-langfuse-ingestion-version": "4",
		}),
		// Langfuse only supports OTLP over HTTP today; the default protocol
		// is protobuf, which is what we want.
		otlptracehttp.WithTimeout(10 * time.Second),
	}
	if endpoint.Scheme == "http" {
		httpOpts = append(httpOpts, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptrace.New(ctx, otlptracehttp.NewClient(httpOpts...))
	if err != nil {
		return nil, false, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		return nil, false, fmt.Errorf("building resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, true, nil
}

// Tracer returns the application tracer. Safe to call before Setup — it will
// return a no-op tracer.
func Tracer() trace.Tracer {
	return otel.Tracer(TracerName)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// SetObservationInput marks the span's input payload (Langfuse "Input" panel).
//
// Pass the string Langfuse should display verbatim — Langfuse stores the OTel
// attribute value as-is, so callers serializing structured data should
// json.Marshal it themselves before calling this helper. Note that
// trace-level input shown in the Langfuse UI is taken from the ROOT span's
// observation input; setting input on a child span only populates that
// observation, not the trace summary.
func SetObservationInput(span trace.Span, input string) {
	span.SetAttributes(attribute.String(AttrObservationInput, input))
}

// SetObservationOutput marks the span's output payload (Langfuse "Output"
// panel). Same as SetObservationInput — see its docstring.
func SetObservationOutput(span trace.Span, output string) {
	span.SetAttributes(attribute.String(AttrObservationOutput, output))
}

// MarkGeneration tags the span as an LLM generation so Langfuse renders it
// with the model/usage view and includes it in cost calculations.
func MarkGeneration(span trace.Span, system, model string) {
	span.SetAttributes(
		attribute.String(AttrObservationType, ObservationGeneration),
		attribute.String("gen_ai.system", system),
		attribute.String("gen_ai.request.model", model),
	)
}

// SetUsage records token counts on a generation span.
func SetUsage(span trace.Span, promptTokens, completionTokens, totalTokens int) {
	attrs := make([]attribute.KeyValue, 0, 3)
	if promptTokens > 0 {
		attrs = append(attrs, attribute.Int("gen_ai.usage.input_tokens", promptTokens))
	}
	if completionTokens > 0 {
		attrs = append(attrs, attribute.Int("gen_ai.usage.output_tokens", completionTokens))
	}
	if totalTokens > 0 {
		attrs = append(attrs, attribute.Int("gen_ai.usage.total_tokens", totalTokens))
	}
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
}

