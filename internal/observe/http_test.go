package observe

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsUseBoundedRouteNotTraceOrTenant(t *testing.T) {
	metrics := NewHTTP("query")
	handler := metrics.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/traces/trace-with-tenant-content", nil))
	result := httptest.NewRecorder()
	metrics.ServeHTTP(result, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), `route="trace"`) || strings.Contains(result.Body.String(), "tenant-content") {
		t.Fatalf("unsafe metrics: %s", result.Body.String())
	}
}
