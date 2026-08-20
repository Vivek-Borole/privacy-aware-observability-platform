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

func TestDecidePrioritizesErrorsThenSlowThenDeterministicSampling(t *testing.T) {
	config := Config{HealthySampleModulo: 1000, SlowThreshold: time.Second}
	errorDecision := Decide("trace-error", []ingest.Envelope{span("trace-error", map[string]string{"http.status_code": "503"})}, config)
	if !errorDecision.Retained || errorDecision.Reason != "retained_error" {
		t.Fatalf("unexpected error decision: %#v", errorDecision)
	}
	slowDecision := Decide("trace-slow", []ingest.Envelope{span("trace-slow", map[string]string{"duration_ms": "1001"})}, config)
	if !slowDecision.Retained || slowDecision.Reason != "retained_slow" {
		t.Fatalf("unexpected slow decision: %#v", slowDecision)
	}
	first := Decide("trace-stable", []ingest.Envelope{span("trace-stable", nil)}, config)
	second := Decide("trace-stable", []ingest.Envelope{span("trace-stable", nil)}, config)
	if first.Retained != second.Retained || first.Reason != second.Reason {
		t.Fatalf("sampling was not deterministic: %#v %#v", first, second)
	}
}
