// Command synthetic-downstream is the Go hop in the fabricated demo topology.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/synthetic"
)

func main() {
	emitter := synthetic.NewEmitter(required("PAOP_INGEST_URL"), required("PAOP_SYNTHETIC_API_KEY"))
	workerURL := required("PAOP_SYNTHETIC_WORKER_URL")
	client := &http.Client{Timeout: 3 * time.Second}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/checkout" {
			http.NotFound(w, r)
			return
		}
		traceID := r.Header.Get("X-Synthetic-Trace-ID")
		if traceID == "" {
			http.Error(w, "trace required", http.StatusBadRequest)
			return
		}
		if err := emitter.Emit(r.Context(), traceID, "postgres checkout lookup", "synthetic-go-downstream", map[string]string{"db.system": "postgresql", "peer.service": "synthetic-async-worker", "messaging.destination": "synthetic-checkout-jobs"}); err != nil {
			slog.Warn("synthetic span unavailable", "errorClass", "ingest_unavailable")
			http.Error(w, "telemetry unavailable", http.StatusServiceUnavailable)
			return
		}
		request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, workerURL+"/jobs", nil)
		if err != nil {
			http.Error(w, "worker unavailable", http.StatusServiceUnavailable)
			return
		}
		request.Header.Set("X-Synthetic-Trace-ID", traceID)
		response, err := client.Do(request)
		if err != nil || response.StatusCode != http.StatusAccepted {
			if response != nil {
				response.Body.Close()
			}
			http.Error(w, "worker unavailable", http.StatusServiceUnavailable)
			return
		}
		response.Body.Close()
		w.WriteHeader(http.StatusAccepted)
	})
	server := &http.Server{Addr: ":8091", Handler: handler, ReadHeaderTimeout: 3 * time.Second}
	slog.Info("synthetic downstream listening", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		os.Exit(1)
	}
}
func required(name string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	slog.Error("configuration missing", "name", name)
	os.Exit(2)
	return ""
}
