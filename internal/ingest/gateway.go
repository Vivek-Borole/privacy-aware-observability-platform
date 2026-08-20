// Package ingest validates and sanitizes tenant telemetry before publication.
package ingest

import (
	"context"
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
	maxSpans       = 100
)

type Event struct {
	EventID      string            `json:"eventId"`
	Signal       string            `json:"signal,omitempty"`
	TraceID      string            `json:"traceId"`
	SpanID       string            `json:"spanId"`
	ParentSpanID string            `json:"parentSpanId,omitempty"`
	Name         string            `json:"name"`
	Attributes   map[string]string `json:"attributes"`
}

type Envelope struct {
	TenantID string            `json:"tenantId"`
	Event    Event             `json:"event"`
	Policy   redaction.Receipt `json:"policy"`
	EventKey string            `json:"eventKey"`
}

type Publisher interface{ Publish(Envelope) error }

// Stager persists sanitized envelopes before tail sampling. Unlike a broker
// publisher, it is the acknowledgement boundary for the live gateway.
type Stager interface {
	Stage(context.Context, Envelope) error
}

// BatchStager makes one accepted OTLP request an all-or-nothing durable
// boundary. Production PostgreSQL staging implements this contract; the small
// Stager interface remains useful for focused unit tests and simple callers.
type BatchStager interface {
	StageBatch(context.Context, []Envelope) error
}

// Authenticator resolves a caller to exactly one tenant. It deliberately does
// not accept a tenant identifier supplied by the untrusted client.
type Authenticator interface {
	Tenant(ctx context.Context, key string) (string, bool, error)
}

// PolicyResolver provides tenant-owned regex rules for the versioned redaction
// policy. The returned expressions are configuration, never telemetry content.
type PolicyResolver interface {
	RedactionPolicy(ctx context.Context, tenantID string) (version string, expressions []string, found bool, err error)
}

// APIKeyAuthenticator stores SHA-256 digests, never raw tenant API keys.
type APIKeyAuthenticator struct{ tenantByDigest map[string]string }

func NewAPIKeyAuthenticator(keys map[string]string) *APIKeyAuthenticator {
	digests := make(map[string]string, len(keys))
	for tenant, key := range keys {
		digests[tenant] = digest(key)
	}
	return &APIKeyAuthenticator{tenantByDigest: digests}
}

func (a *APIKeyAuthenticator) Tenant(_ context.Context, key string) (string, bool, error) {
	candidate := digest(key)
	for tenant, stored := range a.tenantByDigest {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(stored)) == 1 {
			return tenant, true, nil
		}
	}
	return "", false, nil
}

type Gateway struct {
	Authenticator  Authenticator
	Publisher      Publisher
	Stager         Stager
	PolicyVersion  string
	Patterns       []*regexp.Regexp
	PolicyResolver PolicyResolver
}

func (g Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || (r.URL.Path != "/v1/ingest" && r.URL.Path != "/v1/traces" && r.URL.Path != "/v1/logs") {
		http.NotFound(w, r)
		return
	}
	if g.Authenticator == nil || (g.Publisher == nil && g.Stager == nil) {
		http.Error(w, "gateway unavailable", http.StatusServiceUnavailable)
		return
	}
	tenant, ok, err := g.Authenticator.Tenant(r.Context(), r.Header.Get("X-PAOP-API-Key"))
	if err != nil {
		http.Error(w, "gateway unavailable", http.StatusServiceUnavailable)
		return
	}
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer body.Close()
	if r.URL.Path == "/v1/traces" {
		if err := g.acceptOTLPJSON(r.Context(), tenant, body); err != nil {
			if errors.Is(err, ErrInvalidOTLP) {
				http.Error(w, "invalid telemetry envelope", http.StatusBadRequest)
			} else {
				http.Error(w, "durable publish unavailable", http.StatusServiceUnavailable)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{}`))
		return
	}
	if r.URL.Path == "/v1/logs" {
		if err := g.acceptOTLPLogsJSON(r.Context(), tenant, body); err != nil {
			if errors.Is(err, ErrInvalidOTLP) {
				http.Error(w, "invalid telemetry envelope", http.StatusBadRequest)
			} else {
				http.Error(w, "durable publish unavailable", http.StatusServiceUnavailable)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{}`))
		return
	}
	var event Event
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !valid(event) {
		http.Error(w, "invalid telemetry envelope", http.StatusBadRequest)
		return
	}
	if err := g.acceptEvent(r.Context(), tenant, event); err != nil {
		http.Error(w, "durable publish unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"eventId": event.EventID, "status": "accepted"})
}

func (g Gateway) acceptEvent(ctx context.Context, tenant string, event Event) error {
	return g.acceptEvents(ctx, tenant, []Event{event})
}

// acceptEvents sanitizes every event before calling any durable dependency.
// When the production batch stager is installed, no subset of a syntactically
// invalid OTLP payload can become visible as accepted telemetry.
func (g Gateway) acceptEvents(ctx context.Context, tenant string, events []Event) error {
	policyVersion, patterns, err := g.policy(ctx, tenant)
	if err != nil {
		return err
	}
	envelopes := make([]Envelope, 0, len(events))
	for _, event := range events {
		if !valid(event) {
			return ErrInvalidOTLP
		}
		attributes, receipt := redaction.Sanitize(event.Attributes, policyVersion, patterns)
		event.Attributes = attributes
		envelopes = append(envelopes, Envelope{TenantID: tenant, Event: event, Policy: receipt, EventKey: tenant + ":" + event.EventID})
	}
	if g.Stager != nil {
		if batch, ok := g.Stager.(BatchStager); ok {
			return batch.StageBatch(ctx, envelopes)
		}
		for _, envelope := range envelopes {
			if err := g.Stager.Stage(ctx, envelope); err != nil {
				return err
			}
		}
		return nil
	}
	for _, envelope := range envelopes {
		if err := g.Publisher.Publish(envelope); err != nil {
			return err
		}
	}
	return nil
}

func (g Gateway) policy(ctx context.Context, tenant string) (string, []*regexp.Regexp, error) {
	version := g.PolicyVersion
	patterns := append([]*regexp.Regexp(nil), g.Patterns...)
	if g.PolicyResolver == nil {
		return version, patterns, nil
	}
	resolvedVersion, expressions, found, err := g.PolicyResolver.RedactionPolicy(ctx, tenant)
	if err != nil {
		return "", nil, err
	}
	if !found {
		return version, patterns, nil
	}
	if resolvedVersion == "" {
		return "", nil, errors.New("redaction policy version missing")
	}
	for _, expression := range expressions {
		if len(expression) == 0 || len(expression) > 512 {
			return "", nil, errors.New("redaction policy expression invalid")
		}
		compiled, err := regexp.Compile(expression)
		if err != nil {
			return "", nil, errors.New("redaction policy expression invalid")
		}
		patterns = append(patterns, compiled)
	}
	return resolvedVersion, patterns, nil
}

func valid(event Event) bool {
	if event.EventID == "" || event.Name == "" || len(event.Attributes) > maxAttributes || len(event.EventID) > 256 || len(event.TraceID) > 128 || len(event.SpanID) > 128 || len(event.ParentSpanID) > 128 || len(event.Name) > 512 {
		return false
	}
	if event.Signal == "" || event.Signal == "trace" {
		if event.TraceID == "" || event.SpanID == "" {
			return false
		}
	} else if event.Signal != "log" {
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
