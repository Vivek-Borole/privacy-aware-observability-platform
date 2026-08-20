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
	"sync"
	"sync/atomic"
	"time"
)

type result struct {
	TargetPerSecond int     `json:"targetPerSecond"`
	DurationSeconds int     `json:"durationSeconds"`
	Sent            int64   `json:"sent"`
	Accepted        int64   `json:"accepted"`
	Failed          int64   `json:"failed"`
	P95Millis       float64 `json:"p95Millis"`
}

func main() {
	endpoint := flag.String("endpoint", "http://127.0.0.1:18080/v1/traces", "OTLP/HTTP JSON endpoint")
	key := flag.String("api-key", "", "synthetic tenant API key (never written to output)")
	rate := flag.Int("rate", 100, "target spans per second")
	duration := flag.Duration("duration", 10*time.Second, "run duration")
	workers := flag.Int("workers", 4, "concurrent request workers")
	flag.Parse()
	if *key == "" || *rate < 1 || *workers < 1 {
		fmt.Fprintln(os.Stderr, "api-key, positive rate, and positive workers are required")
		os.Exit(2)
	}

	jobs := make(chan int)
	latencies := make(chan time.Duration, *rate*2)
	var sent, accepted, failed int64
	client := &http.Client{Timeout: 5 * time.Second}
	var group sync.WaitGroup
	for range *workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for range jobs {
				start := time.Now()
				atomic.AddInt64(&sent, 1)
				request, err := http.NewRequest(http.MethodPost, *endpoint, bytes.NewReader(spanPayload()))
				if err == nil {
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set("X-PAOP-API-Key", *key)
					var response *http.Response
					response, err = client.Do(request)
					if response != nil {
						response.Body.Close()
						if response.StatusCode == http.StatusAccepted {
							atomic.AddInt64(&accepted, 1)
							latencies <- time.Since(start)
							continue
						}
					}
				}
				atomic.AddInt64(&failed, 1)
			}
		}()
	}
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
	sortFloat64(samples)
	p95 := 0.0
	if len(samples) > 0 {
		p95 = samples[(len(samples)-1)*95/100]
	}
	_ = json.NewEncoder(os.Stdout).Encode(result{TargetPerSecond: *rate, DurationSeconds: int(duration.Seconds()), Sent: sent, Accepted: accepted, Failed: failed, P95Millis: p95})
}

func spanPayload() []byte {
	trace, span := randomID(16), randomID(8)
	spanJSON := map[string]any{"traceId": trace, "spanId": span, "name": "synthetic.checkout", "attributes": []any{
		map[string]any{"key": "service.name", "value": map[string]any{"stringValue": "synthetic-typescript-gateway"}},
		map[string]any{"key": "http.method", "value": map[string]any{"stringValue": "POST"}},
		map[string]any{"key": "db.system", "value": map[string]any{"stringValue": "postgresql"}},
		map[string]any{"key": "messaging.system", "value": map[string]any{"stringValue": "redpanda"}},
		map[string]any{"key": "customer.email", "value": map[string]any{"stringValue": "synthetic.user@example.test"}},
	}}
	payload, _ := json.Marshal(map[string]any{"resourceSpans": []any{map[string]any{"scopeSpans": []any{map[string]any{"spans": []any{spanJSON}}}}}})
	return payload
}
func randomID(bytesLen int) string {
	bytes := make([]byte, bytesLen)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}
func sortFloat64(values []float64) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
