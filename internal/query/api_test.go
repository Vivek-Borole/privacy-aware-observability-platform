package query

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/metadata"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/telemetry"
)

type storeStub struct{ tenant, trace string }
type deletionStub struct {
	tenant, action                string
	deleted, tailDeleted, audited bool
}
type auditReaderStub struct{ tenant string }
type samplingReaderStub struct{ tenant string }

func (s *auditReaderStub) AuditEvents(_ context.Context, tenant string, _ int) ([]metadata.AuditEvent, error) {
	s.tenant = tenant
	return []metadata.AuditEvent{{Action: "safe_action"}}, nil
}
func (s *samplingReaderStub) TailDecisions(_ context.Context, tenant string, _ int) ([]metadata.TailDecision, error) {
	s.tenant = tenant
	return []metadata.TailDecision{{TraceID: "safe-trace", Reason: "retained_error", Retained: true, SpanCount: 2}}, nil
}

func (s *deletionStub) DeleteTenantTelemetry(_ context.Context, tenant string) error {
	s.tenant = tenant
	s.deleted = true
	return nil
}
func (s *deletionStub) DeleteTenantTailData(_ context.Context, tenant string) error {
	s.tenant = tenant
	s.tailDeleted = true
	return nil
}
func (s *deletionStub) RecordAudit(_ context.Context, tenant, action string, _ map[string]string) error {
	s.tenant, s.action, s.audited = tenant, action, true
	return nil
}

func (s *storeStub) QueryTrace(_ context.Context, tenant, trace string) ([]telemetry.Span, error) {
	s.tenant, s.trace = tenant, trace
	return []telemetry.Span{{TraceID: trace, Name: "safe-span"}}, nil
}
func (s *storeStub) QueryUsage(_ context.Context, tenant string) (telemetry.UsageMetrics, error) {
	s.tenant = tenant
	return telemetry.UsageMetrics{WindowHours: 24, SpanCount: 12, TraceCount: 3, LogCount: 4, ErrorCount: 1}, nil
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

func TestDeletionRequiresConfirmationAndUsesAuthenticatedTenant(t *testing.T) {
	store, deletion := &storeStub{}, &deletionStub{}
	api := API{Authenticator: ingest.NewAPIKeyAuthenticator(map[string]string{"tenant-a": "key-a"}), Store: store, Deleter: deletion, TailDeleter: deletion, Auditor: deletion}
	request := httptest.NewRequest(http.MethodPost, "/v1/retention/delete?tenantId=tenant-b", nil)
	request.Header.Set("X-PAOP-API-Key", "key-a")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || deletion.deleted {
		t.Fatalf("unexpected unconfirmed deletion status=%d deleted=%t", response.Code, deletion.deleted)
	}
	request = httptest.NewRequest(http.MethodPost, "/v1/retention/delete?tenantId=tenant-b", nil)
	request.Header.Set("X-PAOP-API-Key", "key-a")
	request.Header.Set("X-PAOP-Delete-Confirm", "DELETE_SANITIZED_TELEMETRY")
	response = httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK || deletion.tenant != "tenant-a" || !deletion.deleted || !deletion.tailDeleted || !deletion.audited || deletion.action != "tenant_telemetry_deletion_requested" {
		t.Fatalf("unsafe deletion: status=%d %#v", response.Code, deletion)
	}
}

func TestAuditUsesAuthenticatedTenantNotClientInput(t *testing.T) {
	store, audit := &storeStub{}, &auditReaderStub{}
	api := API{Authenticator: ingest.NewAPIKeyAuthenticator(map[string]string{"tenant-a": "key-a"}), Store: store, AuditReader: audit}
	request := httptest.NewRequest(http.MethodGet, "/v1/audit?tenantId=tenant-b", nil)
	request.Header.Set("X-PAOP-API-Key", "key-a")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK || audit.tenant != "tenant-a" {
		t.Fatalf("status=%d tenant=%q", response.Code, audit.tenant)
	}
}

func TestSamplingUsesAuthenticatedTenantNotClientInput(t *testing.T) {
	store, sampling := &storeStub{}, &samplingReaderStub{}
	api := API{Authenticator: ingest.NewAPIKeyAuthenticator(map[string]string{"tenant-a": "key-a"}), Store: store, Sampling: sampling}
	request := httptest.NewRequest(http.MethodGet, "/v1/sampling?tenantId=tenant-b", nil)
	request.Header.Set("X-PAOP-API-Key", "key-a")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK || sampling.tenant != "tenant-a" {
		t.Fatalf("status=%d tenant=%q", response.Code, sampling.tenant)
	}
}
