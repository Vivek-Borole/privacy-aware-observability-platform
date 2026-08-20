// Package telemetry persists only sanitized broker envelopes to ClickHouse.
package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
)

type ClickHouse struct {
	endpoint string
	client   *http.Client
}

func NewClickHouse(endpoint string) *ClickHouse {
	return &ClickHouse{endpoint: strings.TrimRight(endpoint, "/"), client: &http.Client{Timeout: 5 * time.Second}}
}

type row struct {
	TenantID       string   `json:"tenant_id"`
	EventKey       string   `json:"event_key"`
	EventID        string   `json:"event_id"`
	TraceID        string   `json:"trace_id"`
	SpanID         string   `json:"span_id"`
	Name           string   `json:"name"`
	AttributesJSON string   `json:"attributes_json"`
	PolicyVersion  string   `json:"policy_version"`
	RedactedPaths  []string `json:"redacted_paths"`
	IngestedAt     string   `json:"ingested_at"`
}

type Span struct {
	EventKey      string            `json:"eventKey"`
	EventID       string            `json:"eventId"`
	TraceID       string            `json:"traceId"`
	SpanID        string            `json:"spanId"`
	Name          string            `json:"name"`
	Attributes    map[string]string `json:"attributes"`
	PolicyVersion string            `json:"policyVersion"`
	RedactedPaths []string          `json:"redactedPaths"`
	IngestedAt    string            `json:"ingestedAt"`
}

func (c *ClickHouse) Persist(ctx context.Context, envelope ingest.Envelope) error {
	attributes, err := json.Marshal(envelope.Event.Attributes)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(row{TenantID: envelope.TenantID, EventKey: envelope.EventKey, EventID: envelope.Event.EventID, TraceID: envelope.Event.TraceID, SpanID: envelope.Event.SpanID, Name: envelope.Event.Name, AttributesJSON: string(attributes), PolicyVersion: envelope.Policy.PolicyVersion, RedactedPaths: envelope.Policy.RedactedPaths, IngestedAt: time.Now().UTC().Format("2006-01-02 15:04:05.000")})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/?query=INSERT%20INTO%20telemetry.spans%20FORMAT%20JSONEachRow", bytes.NewReader(append(payload, '\n')))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("clickhouse status class %d", response.StatusCode/100)
	}
	return nil
}

func (c *ClickHouse) Execute(ctx context.Context, statement string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/", strings.NewReader(statement))
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("clickhouse status class %d", response.StatusCode/100)
	}
	return nil
}

// QueryTrace binds tenant and trace as ClickHouse parameters. Tenant scope is
// never a client-controlled SQL predicate.
func (c *ClickHouse) QueryTrace(ctx context.Context, tenantID, traceID string) ([]Span, error) {
	endpoint, err := url.Parse(c.endpoint + "/")
	if err != nil {
		return nil, err
	}
	values := endpoint.Query()
	values.Set("query", "SELECT event_key,event_id,trace_id,span_id,name,attributes_json,policy_version,redacted_paths,toString(ingested_at) AS ingested_at FROM telemetry.spans FINAL WHERE tenant_id = {tenant:String} AND trace_id = {trace:String} ORDER BY ingested_at ASC FORMAT JSONEachRow")
	values.Set("param_tenant", tenantID)
	values.Set("param_trace", traceID)
	endpoint.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return nil, fmt.Errorf("clickhouse status class %d", response.StatusCode/100)
	}
	decoder := json.NewDecoder(response.Body)
	var spans []Span
	for {
		var stored row
		if err := decoder.Decode(&stored); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		attributes := map[string]string{}
		if err := json.Unmarshal([]byte(stored.AttributesJSON), &attributes); err != nil {
			return nil, err
		}
		spans = append(spans, Span{EventKey: stored.EventKey, EventID: stored.EventID, TraceID: stored.TraceID, SpanID: stored.SpanID, Name: stored.Name, Attributes: attributes, PolicyVersion: stored.PolicyVersion, RedactedPaths: stored.RedactedPaths, IngestedAt: stored.IngestedAt})
	}
	return spans, nil
}

// DeleteOlderThan uses ClickHouse named parameters, keeping the tenant policy
// scope separate from untrusted query input. Mutations are asynchronous in
// ClickHouse; callers record the request as an audit action, not an immediate
// physical-deletion claim.
func (c *ClickHouse) DeleteOlderThan(ctx context.Context, tenantID string, cutoff time.Time) error {
	endpoint, err := url.Parse(c.endpoint + "/")
	if err != nil {
		return err
	}
	values := endpoint.Query()
	values.Set("query", "ALTER TABLE telemetry.spans DELETE WHERE tenant_id = {tenant:String} AND ingested_at < {cutoff:DateTime64(3)}")
	values.Set("param_tenant", tenantID)
	values.Set("param_cutoff", cutoff.UTC().Format("2006-01-02 15:04:05.000"))
	endpoint.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("clickhouse status class %d", response.StatusCode/100)
	}
	return nil
}
