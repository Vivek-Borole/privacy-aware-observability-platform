// Package query serves authenticated, tenant-scoped sanitized trace lookups.
package query

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/telemetry"
)

type TraceStore interface {
	QueryTrace(context.Context, string, string) ([]telemetry.Span, error)
	QueryUsage(context.Context, string) (telemetry.UsageMetrics, error)
}
type API struct {
	Authenticator ingest.Authenticator
	Store         TraceStore
}

var traceID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func (a API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
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
