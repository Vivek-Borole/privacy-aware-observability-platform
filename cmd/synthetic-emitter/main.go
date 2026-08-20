// Command synthetic-emitter generates only fabricated OTLP/HTTP JSON spans.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"net/http"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type result struct {
	StartedAt         string   `json:"startedAt"`
	FinishedAt        string   `json:"finishedAt"`
	TargetPerSecond   int      `json:"targetPerSecond"`
	BatchSize         int      `json:"batchSize"`
	DurationSeconds   int      `json:"durationSeconds"`
	ElapsedMillis     float64  `json:"elapsedMillis"`
	AchievedPerSecond float64  `json:"achievedPerSecond"`
	Sent              int64    `json:"sent"`
	Accepted          int64    `json:"accepted"`
	Failed            int64    `json:"failed"`
	P50Millis         float64  `json:"p50Millis"`
	P95Millis         float64  `json:"p95Millis"`
	P99Millis         float64  `json:"p99Millis"`
	GOOS              string   `json:"goos"`
	GOARCH            string   `json:"goarch"`
	CPUs              int      `json:"cpus"`
	TraceSamples      []string `json:"traceSamples"`
}

func main() {
	endpoint := flag.String("endpoint", "http://127.0.0.1:18080/v1/traces", "OTLP/HTTP JSON endpoint")
	key := flag.String("api-key", "", "synthetic tenant API key (never written to output)")
	rate := flag.Int("rate", 100, "target spans per second")
	duration := flag.Duration("duration", 10*time.Second, "run duration")
	workers := flag.Int("workers", 4, "concurrent request workers")
	batchSize := flag.Int("batch-size", 100, "synthetic spans per OTLP request")
	output := flag.String("output", "", "optional JSON result file")
	sampleLimit := flag.Int("trace-sample-limit", 100, "maximum successful trace IDs retained for later lookup measurement")
	sampleModulo := flag.Int("healthy-sample-modulo", 1, "record trace samples selected by this deterministic healthy-sampling modulo")
	flag.Parse()
	if *key == "" || *rate < 1 || *workers < 1 || *batchSize < 1 || *sampleLimit < 0 || *sampleModulo < 1 {
		fmt.Fprintln(os.Stderr, "api-key, positive rate, workers, batch-size, and healthy-sample-modulo are required")
		os.Exit(2)
	}

	// A single sub-millisecond ticker is not reliable on every host. Queue work
	// in small time-window batches so the reported target is actually emitted.
	jobs := make(chan int, *rate)
	latencies := make(chan time.Duration, *rate*2)
	var sent, accepted, failed int64
	var samplesLock sync.Mutex
	traceSamples := make([]string, 0, *sampleLimit)
	latencySamples := make([]float64, 0, *rate)
	client := &http.Client{Timeout: 5 * time.Second}
	var latencyGroup sync.WaitGroup
	latencyGroup.Add(1)
	go func() {
		defer latencyGroup.Done()
		for latency := range latencies {
			latencySamples = append(latencySamples, float64(latency.Microseconds())/1000)
		}
	}()
	var group sync.WaitGroup
	for range *workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for batch := range jobs {
				start := time.Now()
				atomic.AddInt64(&sent, int64(batch))
				payload, traceIDs := spanPayload(batch)
				request, err := http.NewRequest(http.MethodPost, *endpoint, bytes.NewReader(payload))
				if err == nil {
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set("X-PAOP-API-Key", *key)
					var response *http.Response
					response, err = client.Do(request)
					if response != nil {
						response.Body.Close()
						if response.StatusCode == http.StatusAccepted {
							atomic.AddInt64(&accepted, int64(batch))
							samplesLock.Lock()
							for _, traceID := range traceIDs {
								if len(traceSamples) >= *sampleLimit {
									break
								}
								if sampled(traceID, *sampleModulo) {
									traceSamples = append(traceSamples, traceID)
								}
							}
							samplesLock.Unlock()
							latencies <- time.Since(start)
							continue
						}
					}
				}
				atomic.AddInt64(&failed, int64(batch))
			}
		}()
	}
	startedAt := time.Now().UTC()
	started := time.Now()
	ticker := time.NewTicker(10 * time.Millisecond)
	deadline := time.NewTimer(*duration)
	emitted := 0
	target := int(float64(*rate) * duration.Seconds())
loop:
	for {
		select {
		case <-ticker.C:
			due := int(time.Since(started).Seconds() * float64(*rate))
			if due > target {
				due = target
			}
			for emitted < due {
				batch := min(*batchSize, due-emitted)
				jobs <- batch
				emitted += batch
			}
		case <-deadline.C:
			for emitted < target {
				batch := min(*batchSize, target-emitted)
				jobs <- batch
				emitted += batch
			}
			break loop
		}
	}
	ticker.Stop()
	close(jobs)
	group.Wait()
	close(latencies)
	latencyGroup.Wait()
	sort.Float64s(latencySamples)
	elapsed := time.Since(started)
	report := result{StartedAt: startedAt.Format(time.RFC3339), FinishedAt: time.Now().UTC().Format(time.RFC3339), TargetPerSecond: *rate, BatchSize: *batchSize, DurationSeconds: int(duration.Seconds()), ElapsedMillis: float64(elapsed.Microseconds()) / 1000, AchievedPerSecond: float64(accepted) / elapsed.Seconds(), Sent: sent, Accepted: accepted, Failed: failed, P50Millis: percentile(latencySamples, 50), P95Millis: percentile(latencySamples, 95), P99Millis: percentile(latencySamples, 99), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, CPUs: runtime.NumCPU(), TraceSamples: traceSamples}
	encoded, _ := json.MarshalIndent(report, "", "  ")
	if *output != "" {
		if err := os.WriteFile(*output, append(encoded, '\n'), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	_, _ = os.Stdout.Write(append(encoded, '\n'))
}

func spanPayload(count int) ([]byte, []string) {
	spans := make([]any, 0, count)
	trace := randomID(16)
	for range count {
		span := randomID(8)
		spans = append(spans, map[string]any{"traceId": trace, "spanId": span, "name": "synthetic.checkout", "attributes": []any{
			map[string]any{"key": "service.name", "value": map[string]any{"stringValue": "synthetic-typescript-gateway"}},
			map[string]any{"key": "http.method", "value": map[string]any{"stringValue": "POST"}},
			map[string]any{"key": "db.system", "value": map[string]any{"stringValue": "postgresql"}},
			map[string]any{"key": "messaging.system", "value": map[string]any{"stringValue": "redpanda"}},
			map[string]any{"key": "customer.email", "value": map[string]any{"stringValue": "synthetic.user@example.test"}},
		}})
	}
	payload, _ := json.Marshal(map[string]any{"resourceSpans": []any{map[string]any{"scopeSpans": []any{map[string]any{"spans": spans}}}}})
	return payload, []string{trace}
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

func sampled(traceID string, modulo int) bool {
	h := fnv.New32a()
	_, _ = h.Write([]byte(traceID))
	return int(h.Sum32()%uint32(modulo)) == 0
}
