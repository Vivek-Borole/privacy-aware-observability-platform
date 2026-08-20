// Command tail-pressure creates bounded, fabricated staged traces for the
// Compose pressure test. It never sends network traffic or uses real data.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/metadata"
)

func main() {
	databaseURL := os.Getenv("PAOP_POSTGRES_URL")
	tenantID := flag.String("tenant", "", "synthetic tenant ID")
	count := flag.Int("count", 1001, "number of distinct fabricated traces")
	flag.Parse()
	if databaseURL == "" || *tenantID == "" || *count < 1 {
		log.Fatal("PAOP_POSTGRES_URL, -tenant, and a positive -count are required")
	}
	store, err := metadata.Open(databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for index := 0; index < *count; index++ {
		traceID := fmt.Sprintf("pressure-trace-%04d", index)
		eventID := fmt.Sprintf("pressure-event-%04d", index)
		envelope := ingest.Envelope{
			TenantID: *tenantID,
			EventKey: *tenantID + ":" + eventID,
			Event:    ingest.Event{EventID: eventID, Signal: "trace", TraceID: traceID, SpanID: "pressure-span", Name: "synthetic.pressure", Attributes: map[string]string{"service.name": "synthetic-pressure", "http.status_code": "200"}},
		}
		if err := store.Stage(ctx, envelope); err != nil {
			log.Fatalf("stage synthetic trace %d: %v", index, err)
		}
	}
	fmt.Printf("staged=%d synthetic traces\n", *count)
}
