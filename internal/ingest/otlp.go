package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"strconv"
)

var ErrInvalidOTLP = errors.New("invalid otlp trace request")

// This deliberately constrained OTLP/HTTP JSON adapter accepts trace spans
// only. It converts scalar attributes into the platform's bounded event model;
// unrecognized attribute values are ignored rather than stringified blindly.
type otlpRequest struct {
	ResourceSpans []otlpResourceSpan `json:"resourceSpans"`
}
type otlpResourceSpan struct {
	Resource   otlpResource    `json:"resource"`
	ScopeSpans []otlpScopeSpan `json:"scopeSpans"`
}
type otlpResource struct {
	Attributes []otlpAttribute `json:"attributes"`
}
type otlpScopeSpan struct {
	Spans []otlpSpan `json:"spans"`
}
type otlpLogsRequest struct {
	ResourceLogs []otlpResourceLog `json:"resourceLogs"`
}
type otlpResourceLog struct {
	Resource  otlpResource    `json:"resource"`
	ScopeLogs []otlpScopeLogs `json:"scopeLogs"`
}
type otlpScopeLogs struct {
	LogRecords []otlpLogRecord `json:"logRecords"`
}
type otlpLogRecord struct {
	TimeUnixNano string          `json:"timeUnixNano"`
	TraceID      string          `json:"traceId"`
	SpanID       string          `json:"spanId"`
	SeverityText string          `json:"severityText"`
	Body         otlpValue       `json:"body"`
	Attributes   []otlpAttribute `json:"attributes"`
}
type otlpSpan struct {
	TraceID      string          `json:"traceId"`
	SpanID       string          `json:"spanId"`
	ParentSpanID string          `json:"parentSpanId"`
	Name         string          `json:"name"`
	Attributes   []otlpAttribute `json:"attributes"`
}
type otlpAttribute struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}
type otlpValue struct {
	String *string  `json:"stringValue"`
	Bool   *bool    `json:"boolValue"`
	Int    *string  `json:"intValue"`
	Double *float64 `json:"doubleValue"`
}

func (g Gateway) acceptOTLPJSON(ctx context.Context, tenant string, body io.Reader) error {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var request otlpRequest
	if err := decoder.Decode(&request); err != nil {
		return ErrInvalidOTLP
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ErrInvalidOTLP
	}
	events := make([]Event, 0)
	for _, resourceSpan := range request.ResourceSpans {
		resource := scalarAttributes(resourceSpan.Resource.Attributes)
		for _, scopeSpan := range resourceSpan.ScopeSpans {
			for _, span := range scopeSpan.Spans {
				if len(events) >= maxSpans {
					return ErrInvalidOTLP
				}
				attributes := make(map[string]string, len(resource)+len(span.Attributes)+1)
				for key, value := range resource {
					attributes[key] = value
				}
				for key, value := range scalarAttributes(span.Attributes) {
					attributes[key] = value
				}
				attributes["telemetry.source"] = "otlp_http_json"
				event := Event{EventID: span.TraceID + ":" + span.SpanID, Signal: "trace", TraceID: span.TraceID, SpanID: span.SpanID, ParentSpanID: span.ParentSpanID, Name: span.Name, Attributes: attributes}
				if !valid(event) {
					return ErrInvalidOTLP
				}
				events = append(events, event)
			}
		}
	}
	if len(events) == 0 {
		return ErrInvalidOTLP
	}
	return g.acceptEvents(ctx, tenant, events)
}

// acceptOTLPLogsJSON accepts a constrained OTLP/HTTP JSON log shape. A log may
// lack a trace ID, so its event ID is derived only from stable technical
// identifiers and an ordinal. Its untrusted body remains an attribute and is
// redacted before durable publication.
func (g Gateway) acceptOTLPLogsJSON(ctx context.Context, tenant string, body io.Reader) error {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var request otlpLogsRequest
	if err := decoder.Decode(&request); err != nil {
		return ErrInvalidOTLP
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ErrInvalidOTLP
	}
	events := make([]Event, 0)
	for _, resourceLog := range request.ResourceLogs {
		resource := scalarAttributes(resourceLog.Resource.Attributes)
		for _, scopeLog := range resourceLog.ScopeLogs {
			for _, record := range scopeLog.LogRecords {
				if len(events) >= maxSpans {
					return ErrInvalidOTLP
				}
				attributes := make(map[string]string, len(resource)+len(record.Attributes)+3)
				for key, value := range resource {
					attributes[key] = value
				}
				for key, value := range scalarAttributes(record.Attributes) {
					attributes[key] = value
				}
				if record.SeverityText != "" {
					attributes["log.severity"] = record.SeverityText
				}
				if value, ok := scalarValue(record.Body); ok {
					attributes["log.body"] = value
				}
				attributes["telemetry.source"] = "otlp_http_json"
				identity := record.TraceID + ":" + record.SpanID + ":" + record.TimeUnixNano + ":" + strconv.Itoa(len(events)+1)
				digest := sha256.Sum256([]byte(identity))
				event := Event{EventID: "log-" + fmtHex(digest[:8]), Signal: "log", TraceID: record.TraceID, SpanID: record.SpanID, Name: "log", Attributes: attributes}
				if !valid(event) {
					return ErrInvalidOTLP
				}
				events = append(events, event)
			}
		}
	}
	if len(events) == 0 {
		return ErrInvalidOTLP
	}
	return g.acceptEvents(ctx, tenant, events)
}

func scalarAttributes(attributes []otlpAttribute) map[string]string {
	result := make(map[string]string, len(attributes))
	for _, attribute := range attributes {
		if attribute.Key == "" {
			continue
		}
		if value, ok := scalarValue(attribute.Value); ok {
			result[attribute.Key] = value
		}
	}
	return result
}

func scalarValue(value otlpValue) (string, bool) {
	switch {
	case value.String != nil:
		return *value.String, true
	case value.Bool != nil:
		return strconv.FormatBool(*value.Bool), true
	case value.Int != nil:
		return *value.Int, true
	case value.Double != nil:
		return strconv.FormatFloat(*value.Double, 'g', -1, 64), true
	default:
		return "", false
	}
}

func fmtHex(value []byte) string {
	const alphabet = "0123456789abcdef"
	result := make([]byte, len(value)*2)
	for index, byteValue := range value {
		result[index*2] = alphabet[byteValue>>4]
		result[index*2+1] = alphabet[byteValue&0x0f]
	}
	return string(result)
}
