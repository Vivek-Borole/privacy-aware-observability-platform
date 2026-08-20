package ingest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

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
