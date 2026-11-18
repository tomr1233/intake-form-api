package observability

import (
	"testing"

	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// A dependency bump once moved the SDK's semconv to 1.43.0 while we imported
// 1.40.0: Merge failed with "conflicting Schema URL" and the server exited.
func TestNewResourceMergesWithSDKDefaults(t *testing.T) {
	res, err := newResource("intake-form-api", "test")
	if err != nil {
		t.Fatalf("newResource: %v", err)
	}
	got := map[string]string{}
	for _, kv := range res.Attributes() {
		got[string(kv.Key)] = kv.Value.Emit()
	}
	if got[string(semconv.ServiceNameKey)] != "intake-form-api" {
		t.Errorf("service.name = %q, want intake-form-api", got[string(semconv.ServiceNameKey)])
	}
	if got[string(semconv.ServiceVersionKey)] != "test" {
		t.Errorf("service.version = %q, want test", got[string(semconv.ServiceVersionKey)])
	}
	if got["telemetry.sdk.language"] != "go" {
		t.Errorf("SDK defaults missing: telemetry.sdk.language = %q", got["telemetry.sdk.language"])
	}
}
