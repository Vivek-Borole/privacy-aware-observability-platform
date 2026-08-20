package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQueryUsageBindsTenantAndReturnsCounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("param_tenant"); got != "tenant-a" {
			t.Fatalf("tenant parameter = %q", got)
		}
		if query := r.URL.Query().Get("query"); !strings.Contains(query, "INTERVAL 24 HOUR") || !strings.Contains(query, "tenant_id = {tenant:String}") || !strings.Contains(query, "output_format_json_quote_64bit_integers = 0") {
			t.Fatalf("unexpected query %q", query)
		}
		_, _ = w.Write([]byte(`{"span_count":12,"trace_count":3,"error_count":1,"log_count":4}`))
	}))
	defer server.Close()
	metrics, err := NewClickHouse(server.URL).QueryUsage(context.Background(), "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if metrics.WindowHours != 24 || metrics.SpanCount != 12 || metrics.TraceCount != 3 || metrics.ErrorCount != 1 || metrics.LogCount != 4 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}

func TestQueryDependenciesBindsTenantAndReturnsDerivedEdges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("param_tenant"); got != "tenant-a" {
			t.Fatalf("tenant parameter = %q", got)
		}
		if query := r.URL.Query().Get("query"); !strings.Contains(query, "peer.service") || !strings.Contains(query, "GROUP BY source,target") {
			t.Fatalf("unexpected query %q", query)
		}
		_, _ = w.Write([]byte(`{"source":"gateway","target":"worker","edge_count":2}`))
	}))
	defer server.Close()
	dependencies, err := NewClickHouse(server.URL).QueryDependencies(context.Background(), "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(dependencies) != 1 || dependencies[0].Source != "gateway" || dependencies[0].Target != "worker" || dependencies[0].Count != 2 {
		t.Fatalf("unexpected dependencies: %#v", dependencies)
	}
}

func TestDeleteTenantTelemetryBindsTenant(t *testing.T) {
	var tenant, statement string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant, statement = r.URL.Query().Get("param_tenant"), r.URL.Query().Get("query")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if err := NewClickHouse(server.URL).DeleteTenantTelemetry(context.Background(), "tenant-a"); err != nil {
		t.Fatal(err)
	}
	if tenant != "tenant-a" || !strings.Contains(statement, "DELETE WHERE tenant_id = {tenant:String}") {
		t.Fatalf("unsafe deletion tenant=%q statement=%q", tenant, statement)
	}
}
