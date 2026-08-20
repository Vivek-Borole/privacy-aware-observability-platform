package sampling

import (
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"testing"
	"time"
)

func span(trace string, attrs map[string]string) ingest.Envelope {
	return ingest.Envelope{Event: ingest.Event{TraceID: trace, Attributes: attrs}}
}
func TestErrorTraceIsRetained(t *testing.T) {
	sampler := New(Config{HealthySampleModulo: 1000})
	if decision := sampler.Observe(span("t-error", map[string]string{"http.status_code": "503"}), time.Now()); decision == nil || !decision.Retained || decision.Reason != "retained_error" {
		t.Fatalf("unexpected %#v", decision)
	}
}
func TestPressureEvictionIsExplicit(t *testing.T) {
	sampler := New(Config{MaxTraces: 1})
	if decision := sampler.Observe(span("old", nil), time.Unix(1, 0)); decision != nil {
		t.Fatal(decision)
	}
	decision := sampler.Observe(span("new", nil), time.Unix(2, 0))
	if decision == nil || !decision.Evicted || decision.Reason != "evicted_pressure" {
		t.Fatalf("unexpected %#v", decision)
	}
}
func TestHealthyCompletionIsDeterministic(t *testing.T) {
	sampler := New(Config{HealthySampleModulo: 1})
	sampler.Observe(span("sampled", nil), time.Now())
	decision := sampler.Complete("sampled")
	if decision == nil || !decision.Retained || decision.Reason != "retained_healthy_sample" {
		t.Fatalf("unexpected %#v", decision)
	}
}
