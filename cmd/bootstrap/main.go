// Command bootstrap provisions a local development tenant without storing its raw key.
package main

import (
	"context"
	"log"
	"os"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/metadata"
)

func main() {
	databaseURL, tenantID, apiKey := os.Getenv("PAOP_POSTGRES_URL"), os.Getenv("PAOP_TENANT_ID"), os.Getenv("PAOP_API_KEY")
	if databaseURL == "" || tenantID == "" || apiKey == "" {
		log.Fatal("PAOP_POSTGRES_URL, PAOP_TENANT_ID, and PAOP_API_KEY are required")
	}
	store, err := metadata.Open(databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	if err := store.CreateTenantKey(context.Background(), tenantID, metadata.HashAPIKey(apiKey)); err != nil {
		log.Fatal(err)
	}
	log.Printf("bootstrapped tenant %q with a hashed API key", tenantID)
}
