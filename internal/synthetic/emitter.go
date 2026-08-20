// Package synthetic emits fabricated OTLP/HTTP spans for the local demo only.
package synthetic

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Emitter struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

func NewEmitter(endpoint, apiKey string) *Emitter {
	return &Emitter{endpoint: strings.TrimRight(endpoint, "/"), apiKey: apiKey, client: &http.Client{Timeout: 3 * time.Second}}
}

func (e *Emitter) Emit(ctx context.Context, traceID, name, service string, attributes map[string]string) error {
	if e.endpoint == "" || e.apiKey == "" || traceID == "" || name == "" || service == "" {
		return fmt.Errorf("synthetic telemetry configuration invalid")
	}
	items := make([]map[string]any, 0, len(attributes))
	for key, value := range attributes {
		items = append(items, map[string]any{"key": key, "value": map[string]any{"stringValue": value}})
	}
	payload, err := json.Marshal(map[string]any{"resourceSpans": []any{map[string]any{
		"resource":   map[string]any{"attributes": []any{map[string]any{"key": "service.name", "value": map[string]any{"stringValue": service}}}},
		"scopeSpans": []any{map[string]any{"spans": []any{map[string]any{"traceId": traceID, "spanId": ID(8), "name": name, "attributes": items}}}},
	}}})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint+"/v1/traces", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-PAOP-API-Key", e.apiKey)
	response, err := e.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("synthetic telemetry not accepted")
	}
	return nil
}

func ID(bytesLen int) string {
	bytes := make([]byte, bytesLen)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}
