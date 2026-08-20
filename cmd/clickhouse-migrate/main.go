package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/telemetry"
)

func main() {
	endpoint := os.Getenv("PAOP_CLICKHOUSE_URL")
	if endpoint == "" {
		log.Fatal("PAOP_CLICKHOUSE_URL is required")
	}
	contents, err := os.ReadFile("db/clickhouse/0001_telemetry.sql")
	if err != nil {
		log.Fatal(err)
	}
	store := telemetry.NewClickHouse(endpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	for {
		if err := apply(store, ctx, string(contents)); err == nil {
			log.Print("applied ClickHouse telemetry schema")
			return
		}
		if ctx.Err() != nil {
			log.Fatal("ClickHouse schema unavailable")
		}
		time.Sleep(time.Second)
	}
}

func apply(store *telemetry.ClickHouse, ctx context.Context, statements string) error {
	for _, statement := range splitStatements(statements) {
		if err := store.Execute(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func splitStatements(statements string) []string {
	var result []string
	for _, statement := range strings.Split(statements, ";") {
		if statement = strings.TrimSpace(statement); statement != "" {
			result = append(result, statement)
		}
	}
	return result
}
