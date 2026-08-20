// Package sampling implements bounded, explainable tail decisions.
package sampling

import (
	"hash/fnv"
	"strconv"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
)

type Config struct {
	MaxTraces, MaxSpansPerTrace, HealthySampleModulo int
	SlowThreshold                                    time.Duration
}
type Decision struct {
	TraceID, Reason   string
	Retained, Evicted bool
	Spans             []ingest.Envelope
}
type buffer struct {
	opened time.Time
	spans  []ingest.Envelope
}
type Sampler struct {
	config Config
	traces map[string]*buffer
}

func New(config Config) *Sampler {
	if config.MaxTraces < 1 {
		config.MaxTraces = 1000
	}
	if config.MaxSpansPerTrace < 1 {
		config.MaxSpansPerTrace = 100
	}
	if config.HealthySampleModulo < 1 {
		config.HealthySampleModulo = 100
	}
	if config.SlowThreshold <= 0 {
		config.SlowThreshold = time.Second
	}
	return &Sampler{config: config, traces: map[string]*buffer{}}
}

// Observe buffers a trace until an error/slow condition, pressure eviction, or
// explicit Complete decision. Returned decisions are safe audit metadata: they
// contain IDs and the already-sanitized spans, never removed values.
func (s *Sampler) Observe(envelope ingest.Envelope, now time.Time) *Decision {
	traceID := envelope.Event.TraceID
	b := s.traces[traceID]
	var pressure *Decision
	if b == nil {
		if len(s.traces) >= s.config.MaxTraces {
			pressure = s.evictOldest()
		}
		b = &buffer{opened: now}
		s.traces[traceID] = b
	}
	if len(b.spans) >= s.config.MaxSpansPerTrace {
		delete(s.traces, traceID)
		return &Decision{TraceID: traceID, Reason: "evicted_span_limit", Evicted: true}
	}
	b.spans = append(b.spans, envelope)
	if isError(envelope) {
		delete(s.traces, traceID)
		return &Decision{TraceID: traceID, Reason: "retained_error", Retained: true, Spans: b.spans}
	}
	if isSlow(envelope, s.config.SlowThreshold) {
		delete(s.traces, traceID)
		return &Decision{TraceID: traceID, Reason: "retained_slow", Retained: true, Spans: b.spans}
	}
	if pressure != nil {
		return pressure
	}
	return nil
}

func (s *Sampler) Complete(traceID string) *Decision {
	b := s.traces[traceID]
	if b == nil {
		return nil
	}
	delete(s.traces, traceID)
	if sample(traceID, s.config.HealthySampleModulo) {
		return &Decision{TraceID: traceID, Reason: "retained_healthy_sample", Retained: true, Spans: b.spans}
	}
	return &Decision{TraceID: traceID, Reason: "dropped_healthy_sample", Retained: false}
}
func (s *Sampler) evictOldest() *Decision {
	var id string
	var oldest time.Time
	for traceID, b := range s.traces {
		if id == "" || b.opened.Before(oldest) {
			id, oldest = traceID, b.opened
		}
	}
	delete(s.traces, id)
	return &Decision{TraceID: id, Reason: "evicted_pressure", Evicted: true}
}
func isError(envelope ingest.Envelope) bool {
	status, _ := strconv.Atoi(envelope.Event.Attributes["http.status_code"])
	return status >= 500 || envelope.Event.Attributes["error"] == "true"
}
func isSlow(envelope ingest.Envelope, threshold time.Duration) bool {
	millis, err := strconv.ParseFloat(envelope.Event.Attributes["duration_ms"], 64)
	return err == nil && time.Duration(millis*float64(time.Millisecond)) >= threshold
}
func sample(traceID string, modulo int) bool {
	h := fnv.New32a()
	_, _ = h.Write([]byte(traceID))
	return int(h.Sum32()%uint32(modulo)) == 0
}
