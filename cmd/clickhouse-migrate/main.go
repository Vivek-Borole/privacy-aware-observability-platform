package main

import (
	"context"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/telemetry"
)

func main() {
	endpoint := os.Getenv("PAOP_CLICKHOUSE_URL")
	if endpoint == "" {
		log.Fatal("PAOP_CLICKHOUSE_URL is required")
	}
	entries, err := os.ReadDir("db/clickhouse")
	if err != nil {
		log.Fatal(err)
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, "db/clickhouse/"+entry.Name())
		}
	}
	sort.Strings(files)
	store := telemetry.NewClickHouse(endpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	for {
		if err := applyAll(store, ctx, files); err == nil {
			log.Print("applied ClickHouse telemetry schema")
			return
		}
		if ctx.Err() != nil {
			log.Fatal("ClickHouse schema unavailable")
		}
		time.Sleep(time.Second)
	}
}
func applyAll(store *telemetry.ClickHouse, ctx context.Context, files []string) error {
	for _, file := range files {
		contents, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if err := apply(store, ctx, string(contents)); err != nil {
			return err
		}
	}
	return nil
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
