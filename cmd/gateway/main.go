package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/metadata"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/observe"
	"google.golang.org/grpc"
)

func main() {
	databaseURL := required("PAOP_POSTGRES_URL")
	store, err := metadata.Open(databaseURL)
	if err != nil {
		slog.Error("metadata unavailable", "errorClass", "database_unavailable")
		os.Exit(1)
	}
	defer store.Close()
	patterns := compilePatterns(os.Getenv("PAOP_REDACTION_PATTERNS"))
	policyResolver := ingest.NewCachedPolicyResolver(store, 30*time.Second)
	gateway := ingest.Gateway{Authenticator: store, Stager: store, PolicyVersion: valueOr("PAOP_POLICY_VERSION", "v1"), Patterns: patterns, PolicyResolver: policyResolver}
	metrics := observe.NewHTTP("gateway")
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics)
	mux.Handle("/", metrics.Wrap(gateway))
	server := &http.Server{Addr: valueOr("PAOP_LISTEN_ADDR", ":8080"), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	grpcAddress := valueOr("PAOP_GRPC_LISTEN_ADDR", ":4317")
	listener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		slog.Error("grpc gateway unavailable", "errorClass", "listen_failure")
		os.Exit(1)
	}
	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(1 << 20))
	ingest.RegisterGRPC(grpcServer, gateway)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errors := make(chan error, 2)
	go func() { errors <- server.ListenAndServe() }()
	go func() { errors <- grpcServer.Serve(listener) }()
	slog.Info("ingestion gateway listening", "httpAddress", server.Addr, "grpcAddress", grpcAddress, "policyVersion", gateway.PolicyVersion)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		grpcServer.GracefulStop()
	case err := <-errors:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("gateway stopped", "errorClass", "listen_failure")
			os.Exit(1)
		}
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
func compilePatterns(raw string) []*regexp.Regexp {
	var patterns []*regexp.Regexp
	for _, expression := range strings.Split(raw, ",") {
		if expression != "" {
			patterns = append(patterns, regexp.MustCompile(expression))
		}
	}
	return patterns
}
