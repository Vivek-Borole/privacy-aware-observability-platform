package api

import (
	"encoding/json"
	"os"
	"testing"
)

func TestOpenAPIV1HasRequiredPublicContracts(t *testing.T) {
	data, err := os.ReadFile("openapi-v1.json")
	if err != nil { t.Fatal(err) }
	var document struct { OpenAPI string `json:"openapi"`; Paths map[string]json.RawMessage `json:"paths"` }
	if err := json.Unmarshal(data, &document); err != nil { t.Fatalf("OpenAPI JSON invalid: %v", err) }
	if document.OpenAPI != "3.1.0" { t.Fatalf("unexpected OpenAPI version %q", document.OpenAPI) }
	for _, path := range []string{"/v1/traces", "/v1/traces/{traceId}", "/v1/metrics", "/v1/dependencies", "/v1/audit", "/v1/retention/delete"} {
		if _, ok := document.Paths[path]; !ok { t.Fatalf("required contract path missing: %s", path) }
	}
}
