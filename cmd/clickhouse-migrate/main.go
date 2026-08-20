package main

import (
	"context"
	"log"
	"os"
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
		if err := store.Execute(ctx, string(contents)); err == nil {
			log.Print("applied ClickHouse telemetry schema")
			return
		}
		if ctx.Err() != nil {
			log.Fatal("ClickHouse schema unavailable")
		}
		time.Sleep(time.Second)
	}
}
