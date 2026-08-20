package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/metadata"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/observe"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/query"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/telemetry"
)

func main() {
	store, err := metadata.Open(required("PAOP_POSTGRES_URL"))
	if err != nil {
		slog.Error("metadata unavailable", "errorClass", "database_unavailable")
		os.Exit(1)
	}
	defer store.Close()
	clickhouse := telemetry.NewClickHouse(required("PAOP_CLICKHOUSE_URL"))
	metrics := observe.NewHTTP("query")
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics)
	mux.Handle("/", metrics.Wrap(query.API{Authenticator: store, Store: clickhouse, Deleter: clickhouse, Auditor: store, AuditReader: store}))
	handler := cors(mux, valueOr("PAOP_CONSOLE_ORIGIN", "http://localhost:5173"))
	server := &http.Server{Addr: valueOr("PAOP_QUERY_LISTEN_ADDR", ":8081"), Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	slog.Info("query API listening", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("query API stopped", "errorClass", "listen_failure")
		os.Exit(1)
	}
}
func required(name string) string {
	value := os.Getenv(name)
	if value == "" {
		slog.Error("required configuration missing", "name", name)
		os.Exit(2)
	}
	return value
}
func valueOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func cors(next http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == allowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "X-PAOP-API-Key, Content-Type, X-PAOP-Delete-Confirm")
		}
		if r.Method == http.MethodOptions {
			if r.Header.Get("Origin") != allowedOrigin || (r.Header.Get("Access-Control-Request-Method") != http.MethodGet && r.Header.Get("Access-Control-Request-Method") != http.MethodPost) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
