// Package ingest validates and sanitizes tenant telemetry before publication.
package ingest

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/redaction"
)

const (
	maxBodyBytes   = 1 << 20
	maxAttributes  = 64
	maxStringBytes = 4 << 10
)

type Event struct {
	EventID    string            `json:"eventId"`
	TraceID    string            `json:"traceId"`
	SpanID     string            `json:"spanId"`
	Name       string            `json:"name"`
	Attributes map[string]string `json:"attributes"`
}

type Envelope struct {
	TenantID string            `json:"tenantId"`
	Event    Event             `json:"event"`
	Policy   redaction.Receipt `json:"policy"`
	EventKey string            `json:"eventKey"`
}

type Publisher interface{ Publish(Envelope) error }

// APIKeyAuthenticator stores SHA-256 digests, never raw tenant API keys.
type APIKeyAuthenticator struct{ tenantByDigest map[string]string }

func NewAPIKeyAuthenticator(keys map[string]string) *APIKeyAuthenticator {
	digests := make(map[string]string, len(keys))
	for tenant, key := range keys {
		digests[tenant] = digest(key)
	}
	return &APIKeyAuthenticator{tenantByDigest: digests}
}

func (a *APIKeyAuthenticator) Tenant(key string) (string, bool) {
	candidate := digest(key)
	for tenant, stored := range a.tenantByDigest {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(stored)) == 1 {
			return tenant, true
		}
	}
	return "", false
}

type Gateway struct {
	Authenticator *APIKeyAuthenticator
	Publisher     Publisher
	PolicyVersion string
	Patterns      []*regexp.Regexp
}

func (g Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/ingest" {
		http.NotFound(w, r)
		return
	}
	if g.Authenticator == nil || g.Publisher == nil {
		http.Error(w, "gateway unavailable", http.StatusServiceUnavailable)
		return
	}
	tenant, ok := g.Authenticator.Tenant(r.Header.Get("X-PAOP-API-Key"))
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer body.Close()
	var event Event
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !valid(event) {
		http.Error(w, "invalid telemetry envelope", http.StatusBadRequest)
		return
	}
	attributes, receipt := redaction.Sanitize(event.Attributes, g.PolicyVersion, g.Patterns)
	event.Attributes = attributes
	envelope := Envelope{TenantID: tenant, Event: event, Policy: receipt, EventKey: tenant + ":" + event.EventID}
	if err := g.Publisher.Publish(envelope); err != nil {
		http.Error(w, "durable publish unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"eventId": event.EventID, "status": "accepted"})
}

func valid(event Event) bool {
	if event.EventID == "" || event.TraceID == "" || event.SpanID == "" || event.Name == "" || len(event.Attributes) > maxAttributes {
		return false
	}
	for key, value := range event.Attributes {
		if key == "" || len(key) > 256 || len(value) > maxStringBytes {
			return false
		}
	}
	return true
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

var ErrUnavailable = errors.New("durable publisher unavailable")

// MemoryPublisher is test-only. Production adapters must make the broker the
// acknowledgement boundary before returning nil.
type MemoryPublisher struct {
	Envelopes []Envelope
	Err       error
}

func (p *MemoryPublisher) Publish(envelope Envelope) error {
	if p.Err != nil {
		return p.Err
	}
	p.Envelopes = append(p.Envelopes, envelope)
	return nil
}
