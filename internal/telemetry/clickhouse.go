// Package telemetry persists only sanitized broker envelopes to ClickHouse.
package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
