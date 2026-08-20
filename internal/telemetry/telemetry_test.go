package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/redaction"
)

func TestClickHousePersistsOnlySanitizedEnvelope(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { body = mustRead(t, r); w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	envelope := ingest.Envelope{TenantID: "tenant-a", EventKey: "tenant-a:e1", Event: ingest.Event{EventID: "e1", TraceID: "t1", SpanID: "s1", ParentSpanID: "parent-1", Name: "checkout", Attributes: map[string]string{"authorization": "[REDACTED]", "email": "[REDACTED_EMAIL]"}}, Policy: redaction.Receipt{PolicyVersion: "v1", RedactedPaths: []string{"attributes.authorization", "attributes.email"}}}
	if err := NewClickHouse(server.URL).Persist(context.Background(), envelope); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"Bearer", "seeded-secret", "person@example.test"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("clickhouse request leaked %q", forbidden)
		}
	}
	var stored row
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.EventKey != "tenant-a:e1" || stored.PolicyVersion != "v1" || stored.ParentSpanID != "parent-1" || stored.SignalType != "trace" {
		t.Fatalf("unexpected row %#v", stored)
	}
}

func TestClickHouseLabelsLogRecordsWithoutPersistingRawContent(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { body = mustRead(t, r); w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	envelope := ingest.Envelope{TenantID: "tenant-a", EventKey: "tenant-a:log-1", Event: ingest.Event{EventID: "log-1", Signal: "log", Name: "log", Attributes: map[string]string{"log.body": "[REDACTED_EMAIL]"}}, Policy: redaction.Receipt{PolicyVersion: "v1", RedactedPaths: []string{"attributes.log.body"}}}
	if err := NewClickHouse(server.URL).Persist(context.Background(), envelope); err != nil {
		t.Fatal(err)
	}
	var stored row
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.SignalType != "log" || stored.TraceID != "" || stored.SpanID != "" {
		t.Fatalf("unexpected log row %#v", stored)
	}
}

type fakeLedger struct {
	claim     bool
	persisted int
	claimErr  error
	markErr   error
}

func (f *fakeLedger) ClaimDelivery(context.Context, string, string) (bool, error) {
	return f.claim, f.claimErr
}
func (f *fakeLedger) MarkPersisted(context.Context, string) error      { f.persisted++; return f.markErr }
func (f *fakeLedger) RecordLoss(context.Context, string, string) error { return nil }

type fakeSink struct{ calls int }

func (f *fakeSink) Persist(context.Context, ingest.Envelope) error { f.calls++; return nil }

func TestProcessSkipsPreviouslyPersistedEvent(t *testing.T) {
	ledger, sink := &fakeLedger{claim: false}, &fakeSink{}
	consumer := &Consumer{ledger: ledger, sink: sink}
	if err := consumer.Process(context.Background(), ingest.Envelope{TenantID: "tenant-a", EventKey: "tenant-a:e1"}); err != nil {
		t.Fatal(err)
	}
	if sink.calls != 0 || ledger.persisted != 0 {
		t.Fatalf("duplicate was persisted: sink=%d ledger=%d", sink.calls, ledger.persisted)
	}
}

type failingSink struct{ calls int }

func (f *failingSink) Persist(context.Context, ingest.Envelope) error {
	f.calls++
	return errors.New("storage unavailable")
}

func TestProcessFailureNeverMarksDeliveryPersisted(t *testing.T) {
	ledger, sink := &fakeLedger{claim: true}, &failingSink{}
	consumer := &Consumer{ledger: ledger, sink: sink}
	if err := consumer.Process(context.Background(), ingest.Envelope{TenantID: "tenant-a", EventKey: "tenant-a:e1"}); err == nil {
		t.Fatal("expected persistence error")
	}
	if sink.calls != 1 || ledger.persisted != 0 {
		t.Fatalf("failure incorrectly committed: sink=%d persisted=%d", sink.calls, ledger.persisted)
	}
}

func TestProcessLedgerFailureNeverCallsSink(t *testing.T) {
	ledger, sink := &fakeLedger{claimErr: errors.New("ledger unavailable")}, &fakeSink{}
	consumer := &Consumer{ledger: ledger, sink: sink}
	if err := consumer.Process(context.Background(), ingest.Envelope{TenantID: "tenant-a", EventKey: "tenant-a:e1"}); err == nil {
		t.Fatal("expected ledger error")
	}
	if sink.calls != 0 || ledger.persisted != 0 {
		t.Fatalf("ledger failure leaked to sink: sink=%d persisted=%d", sink.calls, ledger.persisted)
	}
}

func TestRetentionMutationBindsTenantAndCutoff(t *testing.T) {
	var tenant, cutoff, statement string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant, cutoff, statement = r.URL.Query().Get("param_tenant"), r.URL.Query().Get("param_cutoff"), r.URL.Query().Get("query")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if err := NewClickHouse(server.URL).DeleteOlderThan(context.Background(), "tenant-a", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if tenant != "tenant-a" || cutoff == "" || !strings.Contains(statement, "tenant_id = {tenant:String}") {
		t.Fatalf("unsafe retention query tenant=%q cutoff=%q statement=%q", tenant, cutoff, statement)
	}
}

func mustRead(t *testing.T, r *http.Request) string {
	t.Helper()
	var decoded json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	return string(decoded)
}
