// Command synthetic-emitter generates only fabricated OTLP/HTTP JSON spans.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type result struct {
	StartedAt       string   `json:"startedAt"`
	TargetPerSecond int      `json:"targetPerSecond"`
	DurationSeconds int      `json:"durationSeconds"`
	Sent            int64    `json:"sent"`
	Accepted        int64    `json:"accepted"`
	Failed          int64    `json:"failed"`
	P50Millis       float64  `json:"p50Millis"`
	P95Millis       float64  `json:"p95Millis"`
	P99Millis       float64  `json:"p99Millis"`
	GOOS            string   `json:"goos"`
	GOARCH          string   `json:"goarch"`
	CPUs            int      `json:"cpus"`
	TraceSamples    []string `json:"traceSamples"`
}

func main() {
	endpoint := flag.String("endpoint", "http://127.0.0.1:18080/v1/traces", "OTLP/HTTP JSON endpoint")
	key := flag.String("api-key", "", "synthetic tenant API key (never written to output)")
	rate := flag.Int("rate", 100, "target spans per second")
	duration := flag.Duration("duration", 10*time.Second, "run duration")
	workers := flag.Int("workers", 4, "concurrent request workers")
	output := flag.String("output", "", "optional JSON result file")
	sampleLimit := flag.Int("trace-sample-limit", 100, "maximum successful trace IDs retained for later lookup measurement")
	flag.Parse()
	if *key == "" || *rate < 1 || *workers < 1 || *sampleLimit < 0 {
		fmt.Fprintln(os.Stderr, "api-key, positive rate, and positive workers are required")
		os.Exit(2)
	}

	jobs := make(chan int)
	latencies := make(chan time.Duration, *rate*2)
	var sent, accepted, failed int64
	var samplesLock sync.Mutex
	traceSamples := make([]string, 0, *sampleLimit)
	client := &http.Client{Timeout: 5 * time.Second}
	var group sync.WaitGroup
	for range *workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for range jobs {
				start := time.Now()
				atomic.AddInt64(&sent, 1)
				payload, traceID := spanPayload()
				request, err := http.NewRequest(http.MethodPost, *endpoint, bytes.NewReader(payload))
				if err == nil {
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set("X-PAOP-API-Key", *key)
					var response *http.Response
					response, err = client.Do(request)
					if response != nil {
						response.Body.Close()
						if response.StatusCode == http.StatusAccepted {
							atomic.AddInt64(&accepted, 1)
							samplesLock.Lock()
							if len(traceSamples) < *sampleLimit {
								traceSamples = append(traceSamples, traceID)
							}
							samplesLock.Unlock()
							latencies <- time.Since(start)
							continue
						}
					}
				}
				atomic.AddInt64(&failed, 1)
			}
		}()
	}
	startedAt := time.Now().UTC()
	ticker := time.NewTicker(time.Second / time.Duration(*rate))
	deadline := time.NewTimer(*duration)
loop:
	for {
		select {
		case <-ticker.C:
			jobs <- 1
		case <-deadline.C:
			break loop
		}
	}
	ticker.Stop()
	close(jobs)
	group.Wait()
	close(latencies)
	var samples []float64
	for latency := range latencies {
		samples = append(samples, float64(latency.Microseconds())/1000)
	}
	sort.Float64s(samples)
	report := result{StartedAt: startedAt.Format(time.RFC3339), TargetPerSecond: *rate, DurationSeconds: int(duration.Seconds()), Sent: sent, Accepted: accepted, Failed: failed, P50Millis: percentile(samples, 50), P95Millis: percentile(samples, 95), P99Millis: percentile(samples, 99), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, CPUs: runtime.NumCPU(), TraceSamples: traceSamples}
	encoded, _ := json.MarshalIndent(report, "", "  ")
	if *output != "" {
		if err := os.WriteFile(*output, append(encoded, '\n'), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	_, _ = os.Stdout.Write(append(encoded, '\n'))
}

func spanPayload() ([]byte, string) {
	trace, span := randomID(16), randomID(8)
	spanJSON := map[string]any{"traceId": trace, "spanId": span, "name": "synthetic.checkout", "attributes": []any{
		map[string]any{"key": "service.name", "value": map[string]any{"stringValue": "synthetic-typescript-gateway"}},
		map[string]any{"key": "http.method", "value": map[string]any{"stringValue": "POST"}},
		map[string]any{"key": "db.system", "value": map[string]any{"stringValue": "postgresql"}},
		map[string]any{"key": "messaging.system", "value": map[string]any{"stringValue": "redpanda"}},
		map[string]any{"key": "customer.email", "value": map[string]any{"stringValue": "synthetic.user@example.test"}},
	}}
	payload, _ := json.Marshal(map[string]any{"resourceSpans": []any{map[string]any{"scopeSpans": []any{map[string]any{"spans": []any{spanJSON}}}}}})
	return payload, trace
}
func randomID(bytesLen int) string {
	bytes := make([]byte, bytesLen)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}
func percentile(values []float64, percentile int) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[(len(values)-1)*percentile/100]
}
