// Package observe exposes bounded, content-free Prometheus request counters.
package observe

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 3, 10}

type HTTP struct {
	service        string
	mu             sync.Mutex
	counts         map[string]uint64
	durations      map[string][]uint64
	durationSums   map[string]float64
	durationCounts map[string]uint64
}

func NewHTTP(service string) *HTTP {
	return &HTTP{service: service, counts: map[string]uint64{}, durations: map[string][]uint64{}, durationSums: map[string]float64{}, durationCounts: map[string]uint64{}}
}
func (m *HTTP) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		routeName := route(r.URL.Path)
		elapsed := time.Since(started).Seconds()
		m.mu.Lock()
		m.counts[routeName+":"+fmt.Sprint(recorder.status)]++
		if m.durations[routeName] == nil {
			m.durations[routeName] = make([]uint64, len(durationBuckets))
		}
		for index, bucket := range durationBuckets {
			if elapsed <= bucket {
				m.durations[routeName][index]++
			}
		}
		m.durationSums[routeName] += elapsed
		m.durationCounts[routeName]++
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
	_, _ = fmt.Fprintln(w, "# HELP paop_http_request_duration_seconds Bounded request latency without tenant or content labels.")
	_, _ = fmt.Fprintln(w, "# TYPE paop_http_request_duration_seconds histogram")
	for routeName, buckets := range m.durations {
		for index, count := range buckets {
			_, _ = fmt.Fprintf(w, "paop_http_request_duration_seconds_bucket{service=%q,route=%q,le=%q} %d\n", m.service, routeName, fmt.Sprint(durationBuckets[index]), count)
		}
		_, _ = fmt.Fprintf(w, "paop_http_request_duration_seconds_bucket{service=%q,route=%q,le=\"+Inf\"} %d\n", m.service, routeName, m.durationCounts[routeName])
		_, _ = fmt.Fprintf(w, "paop_http_request_duration_seconds_sum{service=%q,route=%q} %g\n", m.service, routeName, m.durationSums[routeName])
		_, _ = fmt.Fprintf(w, "paop_http_request_duration_seconds_count{service=%q,route=%q} %d\n", m.service, routeName, m.durationCounts[routeName])
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
