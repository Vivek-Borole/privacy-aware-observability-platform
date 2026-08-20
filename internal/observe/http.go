// Package observe exposes bounded, content-free Prometheus request counters.
package observe

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type HTTP struct {
	service string
	mu      sync.Mutex
	counts  map[string]uint64
}

func NewHTTP(service string) *HTTP { return &HTTP{service: service, counts: map[string]uint64{}} }
func (m *HTTP) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		m.mu.Lock()
		m.counts[route(r.URL.Path)+":"+fmt.Sprint(recorder.status)]++
		m.mu.Unlock()
	})
}
func (m *HTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	m.mu.Lock()
	defer m.mu.Unlock()
	_, _ = fmt.Fprintln(w, "# HELP paop_http_requests_total Bounded HTTP request outcomes without tenant or content labels.")
	_, _ = fmt.Fprintln(w, "# TYPE paop_http_requests_total counter")
	for key, count := range m.counts {
		parts := strings.Split(key, ":")
		_, _ = fmt.Fprintf(w, "paop_http_requests_total{service=%q,route=%q,code=%q} %d\n", m.service, parts[0], parts[1], count)
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func route(path string) string {
	if strings.HasPrefix(path, "/v1/traces/") {
		return "trace"
	}
	switch path {
	case "/v1/traces":
		return "ingest"
	case "/v1/metrics":
		return "metrics"
	case "/v1/dependencies":
		return "dependencies"
	case "/v1/audit":
		return "audit"
	case "/v1/retention/delete":
		return "deletion"
	default:
		return "other"
	}
}
