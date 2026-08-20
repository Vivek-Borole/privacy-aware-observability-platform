package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/metadata"
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
	server := &http.Server{Addr: valueOr("PAOP_QUERY_LISTEN_ADDR", ":8081"), Handler: query.API{Authenticator: store, Store: telemetry.NewClickHouse(required("PAOP_CLICKHOUSE_URL"))}, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
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
