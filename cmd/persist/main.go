package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

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
	consumer := telemetry.NewConsumer(strings.Split(required("PAOP_KAFKA_BROKERS"), ","), store, telemetry.NewClickHouse(required("PAOP_CLICKHOUSE_URL")))
	defer consumer.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.Info("telemetry persistence worker started")
	if err := consumer.Run(ctx); err != nil && ctx.Err() == nil {
		slog.Error("persistence worker stopped", "errorClass", "consumer_failure")
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
