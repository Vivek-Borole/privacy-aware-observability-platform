package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

type batchStagerStub struct {
	stageCalls int
	batches    [][]Envelope
}

func (s *batchStagerStub) Stage(_ context.Context, _ Envelope) error {
	s.stageCalls++
	return nil
}

func (s *batchStagerStub) StageBatch(_ context.Context, envelopes []Envelope) error {
	s.batches = append(s.batches, append([]Envelope(nil), envelopes...))
	return nil
}

func TestGatewayAuthenticatesThenSanitizesBeforePublish(t *testing.T) {
	publisher := &MemoryPublisher{}
	gateway := Gateway{Authenticator: NewAPIKeyAuthenticator(map[string]string{"tenant-a": "demo-key-only-for-test"}), Publisher: publisher, PolicyVersion: "v1", Patterns: []*regexp.Regexp{regexp.MustCompile(`customer-\d+`)}}
	payload := Event{EventID: "event-1", TraceID: "trace-1", SpanID: "span-1", Name: "checkout", Attributes: map[string]string{"authorization": "Bearer seeded-secret", "user.email": "user@example.test", "customer": "customer-7", "status": "ok"}}
	encoded, _ := json.Marshal(payload)
	request := httptest.NewRequest(http.MethodPost, "/v1/ingest", bytes.NewReader(encoded))
	request.Header.Set("X-PAOP-API-Key", "demo-key-only-for-test")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || len(publisher.Envelopes) != 1 {
		t.Fatalf("status=%d published=%d", response.Code, len(publisher.Envelopes))
	}
	envelope := publisher.Envelopes[0]
	if envelope.TenantID != "tenant-a" || envelope.EventKey != "tenant-a:event-1" {
		t.Fatalf("unexpected identity %#v", envelope)
	}
	sanitized, _ := json.Marshal(envelope)
	for _, secret := range []string{"seeded-secret", "user@example.test", "customer-7", "demo-key-only-for-test"} {
		if strings.Contains(string(sanitized), secret) {
			t.Fatalf("published envelope leaked %q: %s", secret, sanitized)
		}
	}
}

func TestGatewayRejectsUnknownKeyWithoutPublishing(t *testing.T) {
	publisher := &MemoryPublisher{}
	gateway := Gateway{Authenticator: NewAPIKeyAuthenticator(map[string]string{"tenant-a": "valid-key"}), Publisher: publisher}
	request := httptest.NewRequest(http.MethodPost, "/v1/ingest", strings.NewReader(`{"eventId":"x","traceId":"t","spanId":"s","name":"n"}`))
	request.Header.Set("X-PAOP-API-Key", "unknown")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || len(publisher.Envelopes) != 0 {
		t.Fatalf("status=%d published=%d", response.Code, len(publisher.Envelopes))
	}
}

func TestGatewayAcceptsOTLPJSONAndRedactsBeforePublish(t *testing.T) {
	publisher := &MemoryPublisher{}
	gateway := Gateway{Authenticator: NewAPIKeyAuthenticator(map[string]string{"tenant-a": "otlp-test-key"}), Publisher: publisher, Patterns: []*regexp.Regexp{regexp.MustCompile(`customer-\d+`)}}
	payload := `{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"synthetic-gateway"}},{"key":"authorization","value":{"stringValue":"Bearer seeded-token"}}]},"scopeSpans":[{"spans":[{"traceId":"trace-otlp-1","spanId":"span-otlp-1","name":"GET /checkout","attributes":[{"key":"customer","value":{"stringValue":"customer-7"}},{"key":"email","value":{"stringValue":"person@example.test"}}]}]}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(payload))
	request.Header.Set("X-PAOP-API-Key", "otlp-test-key")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || len(publisher.Envelopes) != 1 {
		t.Fatalf("status=%d published=%d", response.Code, len(publisher.Envelopes))
	}
	serialized, _ := json.Marshal(publisher.Envelopes[0])
	for _, forbidden := range []string{"seeded-token", "person@example.test", "customer-7"} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("sanitized OTLP envelope leaked %q", forbidden)
		}
	}
}

func TestGatewayPreservesTechnicalParentSpanIdentifier(t *testing.T) {
	publisher := &MemoryPublisher{}
	gateway := Gateway{Authenticator: NewAPIKeyAuthenticator(map[string]string{"tenant": "key"}), Publisher: publisher, PolicyVersion: "v1"}
	payload := `{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"trace-parent-1","spanId":"span-child-1","parentSpanId":"span-parent-1","name":"child"}]}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(payload))
	request.Header.Set("X-PAOP-API-Key", "key")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || len(publisher.Envelopes) != 1 || publisher.Envelopes[0].Event.ParentSpanID != "span-parent-1" {
		t.Fatalf("causal relationship lost: status=%d envelopes=%#v", response.Code, publisher.Envelopes)
	}
}

func TestGatewayRejectsInvalidLaterOTLPSpanWithoutPublishingPrefix(t *testing.T) {
	publisher := &MemoryPublisher{}
	gateway := Gateway{Authenticator: NewAPIKeyAuthenticator(map[string]string{"tenant": "key"}), Publisher: publisher}
	payload := `{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"valid-trace","spanId":"valid-span","name":"valid"},{"traceId":"invalid-trace","spanId":"","name":"invalid"}]}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(payload))
	request.Header.Set("X-PAOP-API-Key", "key")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || len(publisher.Envelopes) != 0 {
		t.Fatalf("invalid OTLP request staged a prefix: status=%d envelopes=%#v", response.Code, publisher.Envelopes)
	}
}

func TestGatewayUsesBatchStagingForOneValidOTLPRequest(t *testing.T) {
	stager := &batchStagerStub{}
	gateway := Gateway{Authenticator: NewAPIKeyAuthenticator(map[string]string{"tenant": "key"}), Stager: stager}
	payload := `{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"trace-1","spanId":"span-1","name":"one"},{"traceId":"trace-1","spanId":"span-2","name":"two"}]}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(payload))
	request.Header.Set("X-PAOP-API-Key", "key")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || stager.stageCalls != 0 || len(stager.batches) != 1 || len(stager.batches[0]) != 2 {
		t.Fatalf("expected one atomic batch: status=%d stage=%d batches=%#v", response.Code, stager.stageCalls, stager.batches)
	}
}

func TestGatewayAcceptsOTLPLogsAndRedactsBeforePublish(t *testing.T) {
	publisher := &MemoryPublisher{}
	gateway := Gateway{Authenticator: NewAPIKeyAuthenticator(map[string]string{"tenant-a": "log-test-key"}), Publisher: publisher, Patterns: []*regexp.Regexp{regexp.MustCompile(`customer-\d+`)}}
	payload := `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"synthetic-gateway"}},{"key":"authorization","value":{"stringValue":"Bearer seeded-token"}}]},"scopeLogs":[{"logRecords":[{"timeUnixNano":"123","traceId":"trace-log-1","spanId":"span-log-1","severityText":"ERROR","body":{"stringValue":"person@example.test customer-7"},"attributes":[{"key":"cookie","value":{"stringValue":"session=seeded-cookie"}}]}]}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(payload))
	request.Header.Set("X-PAOP-API-Key", "log-test-key")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || len(publisher.Envelopes) != 1 {
		t.Fatalf("status=%d published=%d", response.Code, len(publisher.Envelopes))
	}
	envelope := publisher.Envelopes[0]
	if envelope.Event.Signal != "log" || envelope.Event.TraceID != "trace-log-1" || envelope.Event.SpanID != "span-log-1" {
		t.Fatalf("unexpected log identity: %#v", envelope.Event)
	}
	serialized, _ := json.Marshal(envelope)
	for _, forbidden := range []string{"seeded-token", "person@example.test", "customer-7", "seeded-cookie"} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("sanitized log envelope leaked %q", forbidden)
		}
	}
}

func TestGatewayRejectsLogWithNoUsableContent(t *testing.T) {
	publisher := &MemoryPublisher{}
	gateway := Gateway{Authenticator: NewAPIKeyAuthenticator(map[string]string{"tenant-a": "log-test-key"}), Publisher: publisher}
	request := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{}]}]}]}`))
	request.Header.Set("X-PAOP-API-Key", "log-test-key")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || len(publisher.Envelopes) != 1 || publisher.Envelopes[0].Event.EventID == "" {
		t.Fatalf("a bounded content-free OTLP log must receive a safe event ID: status=%d envelopes=%#v", response.Code, publisher.Envelopes)
	}
}
