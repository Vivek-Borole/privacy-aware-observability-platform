package ingest

import (
	"context"
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
type otlpSpan struct {
	TraceID    string          `json:"traceId"`
	SpanID     string          `json:"spanId"`
	Name       string          `json:"name"`
	Attributes []otlpAttribute `json:"attributes"`
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
	count := 0
	for _, resourceSpan := range request.ResourceSpans {
		resource := scalarAttributes(resourceSpan.Resource.Attributes)
		for _, scopeSpan := range resourceSpan.ScopeSpans {
			for _, span := range scopeSpan.Spans {
				count++
				attributes := make(map[string]string, len(resource)+len(span.Attributes)+1)
				for key, value := range resource {
					attributes[key] = value
				}
				for key, value := range scalarAttributes(span.Attributes) {
					attributes[key] = value
				}
				attributes["telemetry.source"] = "otlp_http_json"
				event := Event{EventID: span.TraceID + ":" + span.SpanID, TraceID: span.TraceID, SpanID: span.SpanID, Name: span.Name, Attributes: attributes}
				if count > maxSpans || !valid(event) {
					return ErrInvalidOTLP
				}
				if err := g.acceptEvent(ctx, tenant, event); err != nil {
					return err
				}
			}
		}
	}
	if count == 0 {
		return ErrInvalidOTLP
	}
	return nil
}

func scalarAttributes(attributes []otlpAttribute) map[string]string {
	result := make(map[string]string, len(attributes))
	for _, attribute := range attributes {
		if attribute.Key == "" {
			continue
		}
		switch {
		case attribute.Value.String != nil:
			result[attribute.Key] = *attribute.Value.String
		case attribute.Value.Bool != nil:
			result[attribute.Key] = strconv.FormatBool(*attribute.Value.Bool)
		case attribute.Value.Int != nil:
			result[attribute.Key] = *attribute.Value.Int
		case attribute.Value.Double != nil:
			result[attribute.Key] = strconv.FormatFloat(*attribute.Value.Double, 'g', -1, 64)
		}
	}
	return result
}
