package main

import (
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/broker"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/metadata"
)

func main() {
	databaseURL := required("PAOP_POSTGRES_URL")
	store, err := metadata.Open(databaseURL)
	if err != nil {
		slog.Error("metadata unavailable", "errorClass", "database_unavailable")
		os.Exit(1)
	}
	defer store.Close()
	publisher := broker.NewKafkaPublisher(splitRequired("PAOP_KAFKA_BROKERS"))
	defer publisher.Close()

	patterns := compilePatterns(os.Getenv("PAOP_REDACTION_PATTERNS"))
	gateway := ingest.Gateway{Authenticator: store, Publisher: publisher, PolicyVersion: valueOr("PAOP_POLICY_VERSION", "v1"), Patterns: patterns}
	server := &http.Server{Addr: valueOr("PAOP_LISTEN_ADDR", ":8080"), Handler: gateway, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	slog.Info("ingestion gateway listening", "address", server.Addr, "policyVersion", gateway.PolicyVersion)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("gateway stopped", "errorClass", "listen_failure")
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
func splitRequired(name string) []string { return strings.Split(required(name), ",") }
func compilePatterns(raw string) []*regexp.Regexp {
	var patterns []*regexp.Regexp
	for _, expression := range strings.Split(raw, ",") {
		if expression != "" {
			patterns = append(patterns, regexp.MustCompile(expression))
		}
	}
	return patterns
}
