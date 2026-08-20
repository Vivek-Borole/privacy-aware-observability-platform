package query

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/telemetry"
)

type storeStub struct{ tenant, trace string }

func (s *storeStub) QueryTrace(_ context.Context, tenant, trace string) ([]telemetry.Span, error) {
	s.tenant, s.trace = tenant, trace
	return []telemetry.Span{{TraceID: trace, Name: "safe-span"}}, nil
}
func (s *storeStub) QueryUsage(_ context.Context, tenant string) (telemetry.UsageMetrics, error) {
	s.tenant = tenant
	return telemetry.UsageMetrics{WindowHours: 24, SpanCount: 12, TraceCount: 3, ErrorCount: 1}, nil
}
func (s *storeStub) QueryDependencies(_ context.Context, tenant string) ([]telemetry.Dependency, error) {
	s.tenant = tenant
	return []telemetry.Dependency{{Source: "safe-a", Target: "safe-b", Count: 1}}, nil
}

func TestAPIUsesAuthenticatedTenantNotClientInput(t *testing.T) {
	store := &storeStub{}
	api := API{Authenticator: ingest.NewAPIKeyAuthenticator(map[string]string{"tenant-a": "key-a"}), Store: store}
	request := httptest.NewRequest(http.MethodGet, "/v1/traces/trace-1?tenantId=tenant-b", nil)
	request.Header.Set("X-PAOP-API-Key", "key-a")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK || store.tenant != "tenant-a" || store.trace != "trace-1" {
		t.Fatalf("status=%d tenant=%q trace=%q", response.Code, store.tenant, store.trace)
	}
}

func TestMetricsUsesAuthenticatedTenantNotClientInput(t *testing.T) {
	store := &storeStub{}
	api := API{Authenticator: ingest.NewAPIKeyAuthenticator(map[string]string{"tenant-a": "key-a"}), Store: store}
	request := httptest.NewRequest(http.MethodGet, "/v1/metrics?tenantId=tenant-b", nil)
	request.Header.Set("X-PAOP-API-Key", "key-a")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK || store.tenant != "tenant-a" {
		t.Fatalf("status=%d tenant=%q", response.Code, store.tenant)
	}
}

func TestDependenciesUseAuthenticatedTenantNotClientInput(t *testing.T) {
	store := &storeStub{}
	api := API{Authenticator: ingest.NewAPIKeyAuthenticator(map[string]string{"tenant-a": "key-a"}), Store: store}
	request := httptest.NewRequest(http.MethodGet, "/v1/dependencies?tenantId=tenant-b", nil)
	request.Header.Set("X-PAOP-API-Key", "key-a")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK || store.tenant != "tenant-a" {
		t.Fatalf("status=%d tenant=%q", response.Code, store.tenant)
	}
}
