package tail

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/metadata"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/sampling"
)

type storeStub struct {
	leases     []metadata.TailTraceLease
	envelopes  []ingest.Envelope
	outbox     []metadata.TailOutboxMessage
	decisions  []sampling.Decision
	published  []string
	decisionOK bool
}

func (s *storeStub) ClaimTailTraces(context.Context, string, time.Duration, time.Duration, int) ([]metadata.TailTraceLease, error) {
	leases := s.leases
	s.leases = nil
	return leases, nil
}
func (s *storeStub) LoadTailTrace(context.Context, metadata.TailTraceLease) ([]ingest.Envelope, error) {
	return s.envelopes, nil
}
func (s *storeStub) RecordTailDecision(_ context.Context, _ string, _ metadata.TailTraceLease, decision sampling.Decision) (bool, error) {
	s.decisions = append(s.decisions, decision)
	return s.decisionOK, nil
}
func (s *storeStub) ClaimTailOutbox(context.Context, string, time.Duration, int) ([]metadata.TailOutboxMessage, error) {
	return s.outbox, nil
}
func (s *storeStub) MarkTailOutboxPublished(_ context.Context, _ string, eventKey string) error {
	s.published = append(s.published, eventKey)
	return nil
}

type publisherStub struct {
	published []ingest.Envelope
	err       error
}

func (p *publisherStub) Publish(envelope ingest.Envelope) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, envelope)
	return nil
}

func TestProcessRetainsErrorTraceAndPublishesOutbox(t *testing.T) {
	envelope := ingest.Envelope{EventKey: "tenant:error", Event: ingest.Event{TraceID: "trace-error", Attributes: map[string]string{"http.status_code": "503"}}}
	store := &storeStub{leases: []metadata.TailTraceLease{{TenantID: "tenant", TraceID: "trace-error", Generation: 1}}, envelopes: []ingest.Envelope{envelope}, outbox: []metadata.TailOutboxMessage{{EventKey: envelope.EventKey, Envelope: envelope}}, decisionOK: true}
	publisher := &publisherStub{}
	runner := Runner{Store: store, Publisher: publisher, Config: Config{Sampling: sampling.Config{HealthySampleModulo: 1000}}}
	if err := runner.Process(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.decisions) != 1 || store.decisions[0].Reason != "retained_error" || !store.decisions[0].Retained {
		t.Fatalf("unexpected decisions: %#v", store.decisions)
	}
	if len(publisher.published) != 1 || len(store.published) != 1 || store.published[0] != envelope.EventKey {
		t.Fatalf("outbox was not committed exactly after broker success: published=%#v marked=%#v", publisher.published, store.published)
	}
}

func TestProcessDoesNotMarkOutboxWhenBrokerFails(t *testing.T) {
	envelope := ingest.Envelope{EventKey: "tenant:effect"}
	store := &storeStub{outbox: []metadata.TailOutboxMessage{{EventKey: envelope.EventKey, Envelope: envelope}}, decisionOK: true}
	runner := Runner{Store: store, Publisher: &publisherStub{err: errors.New("broker unavailable")}}
	if err := runner.Process(context.Background()); err == nil {
		t.Fatal("expected broker failure")
	}
	if len(store.published) != 0 {
		t.Fatalf("outbox was marked after failed broker publish: %#v", store.published)
	}
}

func TestProcessRecordsDroppedHealthyDecision(t *testing.T) {
	config := sampling.Config{HealthySampleModulo: 100}
	traceID := "trace-healthy"
	for sampling.Decide(traceID, []ingest.Envelope{{Event: ingest.Event{TraceID: traceID}}}, config).Retained {
		traceID += "-next"
	}
	envelope := ingest.Envelope{EventKey: "tenant:healthy", Event: ingest.Event{TraceID: traceID}}
	store := &storeStub{leases: []metadata.TailTraceLease{{TenantID: "tenant", TraceID: traceID, Generation: 1}}, envelopes: []ingest.Envelope{envelope}, decisionOK: true}
	runner := Runner{Store: store, Publisher: &publisherStub{}, Config: Config{Sampling: config}}
	if err := runner.Process(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.decisions) != 1 || store.decisions[0].Retained || store.decisions[0].Reason != "dropped_healthy_sample" {
		t.Fatalf("unexpected healthy decision: %#v", store.decisions)
	}
}
