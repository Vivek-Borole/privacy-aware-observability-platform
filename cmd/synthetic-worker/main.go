// Command synthetic-worker represents an asynchronous fabricated consumer.
package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/synthetic"
)

func main() {
	emitter := synthetic.NewEmitter(required("PAOP_INGEST_URL"), required("PAOP_SYNTHETIC_API_KEY"))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/jobs" {
			http.NotFound(w, r)
			return
		}
		traceID := r.Header.Get("X-Synthetic-Trace-ID")
		if traceID == "" {
			http.Error(w, "trace required", http.StatusBadRequest)
			return
		}
		if err := emitter.Emit(r.Context(), traceID, "process checkout job", "synthetic-async-worker", map[string]string{"messaging.system": "redpanda", "messaging.operation": "process", "peer.service": "synthetic-go-downstream"}); err != nil {
			slog.Warn("synthetic span unavailable", "errorClass", "ingest_unavailable")
			http.Error(w, "telemetry unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
	server := &http.Server{Addr: ":8092", Handler: handler, ReadHeaderTimeout: 3 * time.Second}
	slog.Info("synthetic worker listening", "address", server.Addr)
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
