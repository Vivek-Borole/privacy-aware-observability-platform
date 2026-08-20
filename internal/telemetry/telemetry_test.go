package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/redaction"
)

func TestClickHousePersistsOnlySanitizedEnvelope(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { body = mustRead(t, r); w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	envelope := ingest.Envelope{TenantID: "tenant-a", EventKey: "tenant-a:e1", Event: ingest.Event{EventID: "e1", TraceID: "t1", SpanID: "s1", Name: "checkout", Attributes: map[string]string{"authorization": "[REDACTED]", "email": "[REDACTED_EMAIL]"}}, Policy: redaction.Receipt{PolicyVersion: "v1", RedactedPaths: []string{"attributes.authorization", "attributes.email"}}}
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
	if stored.EventKey != "tenant-a:e1" || stored.PolicyVersion != "v1" {
		t.Fatalf("unexpected row %#v", stored)
	}
}

type fakeLedger struct {
	claim     bool
	persisted int
}

func (f *fakeLedger) ClaimDelivery(context.Context, string, string) (bool, error) {
	return f.claim, nil
}
func (f *fakeLedger) MarkPersisted(context.Context, string) error      { f.persisted++; return nil }
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

func mustRead(t *testing.T, r *http.Request) string {
	t.Helper()
	var decoded json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	return string(decoded)
}
