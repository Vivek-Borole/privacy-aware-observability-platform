package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/broker"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/metadata"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/sampling"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/tail"
	"github.com/jackc/pgx/v5/pgconn"
)

func main() {
	store, err := metadata.Open(required("PAOP_POSTGRES_URL"))
	if err != nil {
		slog.Error("metadata unavailable", "errorClass", "database_unavailable")
		os.Exit(1)
	}
	defer store.Close()
	publisher := broker.NewKafkaPublisher(strings.Split(required("PAOP_KAFKA_BROKERS"), ","))
	defer publisher.Close()
	quiet, err := duration("PAOP_TAIL_QUIET", 2*time.Second)
	if err != nil {
		slog.Error("invalid tail configuration", "errorClass", "configuration_invalid")
		os.Exit(2)
	}
	modulo, err := integer("PAOP_HEALTHY_SAMPLE_MODULO", 100)
	if err != nil || modulo < 1 {
		slog.Error("invalid tail configuration", "errorClass", "configuration_invalid")
		os.Exit(2)
	}
	runner := tail.Runner{Store: store, Publisher: publisher, Config: tail.Config{Owner: valueOr("PAOP_TAIL_OWNER", "tailer"), Interval: valueOr("PAOP_TAIL_INTERVAL", "1s"), Quiet: quiet, Sampling: sampling.Config{HealthySampleModulo: modulo, SlowThreshold: time.Second}}}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.Info("durable tail sampler started", "healthySampleModulo", modulo)
	if err := runner.Run(ctx); err != nil && ctx.Err() == nil {
		slog.Error("tail sampler stopped", "errorClass", errorClass(err))
		os.Exit(1)
	}
}

func errorClass(err error) string {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		return "postgres_" + databaseError.Code
	}
	return "tail_failure"
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
func duration(name string, fallback time.Duration) (time.Duration, error) {
	if value := os.Getenv(name); value != "" {
		return time.ParseDuration(value)
	}
	return fallback, nil
}
func integer(name string, fallback int) (int, error) {
	if value := os.Getenv(name); value != "" {
		return strconv.Atoi(value)
	}
	return fallback, nil
}
