package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/metadata"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/telemetry"
)

func main() {
	store, err := metadata.Open(required("PAOP_POSTGRES_URL"))
	if err != nil {
		slog.Error("metadata unavailable", "errorClass", "database_unavailable")
		os.Exit(1)
	}
	defer store.Close()
	policies, err := store.RetentionPolicies(context.Background())
	if err != nil {
		slog.Error("retention policies unavailable", "errorClass", "database_unavailable")
		os.Exit(1)
	}
	clickhouse := telemetry.NewClickHouse(required("PAOP_CLICKHOUSE_URL"))
	for _, policy := range policies {
		cutoff := time.Now().UTC().AddDate(0, 0, -policy.Days)
		if err := clickhouse.DeleteOlderThan(context.Background(), policy.TenantID, cutoff); err != nil {
			slog.Error("retention mutation request failed", "tenant", policy.TenantID, "errorClass", "clickhouse_unavailable")
			os.Exit(1)
		}
		if err := store.DeleteTailDataBefore(context.Background(), policy.TenantID, cutoff); err != nil {
			slog.Error("tail retention cleanup failed", "tenant", policy.TenantID, "errorClass", "database_unavailable")
			os.Exit(1)
		}
		slog.Info("retention mutation requested", "tenant", policy.TenantID, "days", policy.Days)
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
