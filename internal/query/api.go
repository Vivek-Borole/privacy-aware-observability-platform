// Package query serves authenticated, tenant-scoped sanitized trace lookups.
package query

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/metadata"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/telemetry"
)

type TraceStore interface {
	QueryTrace(context.Context, string, string) ([]telemetry.Span, error)
	QueryUsage(context.Context, string) (telemetry.UsageMetrics, error)
	QueryDependencies(context.Context, string) ([]telemetry.Dependency, error)
}
type TenantDeleter interface {
	DeleteTenantTelemetry(context.Context, string) error
}
type AuditSink interface {
	RecordAudit(context.Context, string, string, map[string]string) error
}
type AuditReader interface {
	AuditEvents(context.Context, string, int) ([]metadata.AuditEvent, error)
}
type API struct {
	Authenticator ingest.Authenticator
	Store         TraceStore
	Deleter       TenantDeleter
	Auditor       AuditSink
	AuditReader   AuditReader
}

var traceID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func (a API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if a.Authenticator == nil || a.Store == nil {
		http.Error(w, "query unavailable", http.StatusServiceUnavailable)
		return
	}
	tenant, ok, err := a.Authenticator.Tenant(r.Context(), r.Header.Get("X-PAOP-API-Key"))
	if err != nil {
		http.Error(w, "query unavailable", http.StatusServiceUnavailable)
		return
	}
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/v1/retention/delete" {
		if r.Method != http.MethodPost || r.Header.Get("X-PAOP-Delete-Confirm") != "DELETE_SANITIZED_TELEMETRY" {
			http.Error(w, "explicit deletion confirmation required", http.StatusBadRequest)
			return
		}
		if a.Deleter == nil || a.Auditor == nil {
			http.Error(w, "deletion unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := a.Deleter.DeleteTenantTelemetry(r.Context(), tenant); err != nil {
			http.Error(w, "deletion unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := a.Auditor.RecordAudit(r.Context(), tenant, "tenant_telemetry_deletion_requested", map[string]string{"scope": "all_sanitized_telemetry", "state": "clickhouse_mutation_requested"}); err != nil {
			http.Error(w, "deletion audit unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"state": "mutation_requested", "scope": "all_sanitized_telemetry"})
		return
	}
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/v1/metrics" {
		metrics, err := a.Store.QueryUsage(r.Context(), tenant)
		if err != nil {
			http.Error(w, "query unavailable", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(metrics)
		return
	}
	if r.URL.Path == "/v1/dependencies" {
		dependencies, err := a.Store.QueryDependencies(r.Context(), tenant)
		if err != nil {
			http.Error(w, "query unavailable", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"windowHours": 24, "dependencies": dependencies})
		return
	}
	if r.URL.Path == "/v1/audit" {
		if a.AuditReader == nil {
			http.Error(w, "audit unavailable", http.StatusServiceUnavailable)
			return
		}
		events, err := a.AuditReader.AuditEvents(r.Context(), tenant, 100)
		if err != nil {
			http.Error(w, "audit unavailable", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"events": events})
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/v1/traces/") {
		http.NotFound(w, r)
		return
	}
	trace := strings.TrimPrefix(r.URL.Path, "/v1/traces/")
	if !traceID.MatchString(trace) {
		http.Error(w, "invalid trace id", http.StatusBadRequest)
		return
	}
	spans, err := a.Store.QueryTrace(r.Context(), tenant, trace)
	if err != nil {
		http.Error(w, "query unavailable", http.StatusServiceUnavailable)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"traceId": trace, "spans": spans})
}
