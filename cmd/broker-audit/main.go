// Command broker-audit verifies that known synthetic raw markers are absent
// from the durable telemetry topic. It never prints broker payloads: the
// contents are inspected only in memory so the evidence command cannot create
// the very leak it is meant to detect.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/broker"
	"github.com/segmentio/kafka-go"
)

func main() {
	brokers := flag.String("brokers", os.Getenv("PAOP_KAFKA_BROKERS"), "comma-separated Kafka-compatible brokers")
	topic := flag.String("topic", broker.TelemetryTopic, "topic to audit")
	forbidden := flag.String("forbidden", "", "comma-separated raw marker strings that must not occur")
	quiet := flag.Duration("quiet", 2*time.Second, "stop after this interval without another message")
	flag.Parse()

	if strings.TrimSpace(*brokers) == "" || strings.TrimSpace(*forbidden) == "" {
		fmt.Fprintln(os.Stderr, "brokers and forbidden markers are required")
		os.Exit(2)
	}

	markers := splitNonEmpty(*forbidden)
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     splitNonEmpty(*brokers),
		Topic:       *topic,
		Partition:   0,
		StartOffset: kafka.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})
	defer reader.Close()

	count := 0
	for {
		ctx, cancel := context.WithTimeout(context.Background(), *quiet)
		message, err := reader.ReadMessage(ctx)
		cancel()
		if err != nil {
			if ctxErr := context.DeadlineExceeded; strings.Contains(err.Error(), ctxErr.Error()) || count > 0 {
				break
			}
			fmt.Fprintf(os.Stderr, "broker audit failed before receiving telemetry: %v\n", err)
			os.Exit(1)
		}
		count++
		payload := string(message.Value)
		for _, marker := range markers {
			if strings.Contains(payload, marker) {
				fmt.Fprintln(os.Stderr, "broker audit failed: a forbidden raw marker was found")
				os.Exit(1)
			}
		}
	}

	if count == 0 {
		fmt.Fprintln(os.Stderr, "broker audit failed: no telemetry messages were observed")
		os.Exit(1)
	}
	fmt.Printf("broker audit passed: inspected=%d messages; raw markers absent\n", count)
}

func splitNonEmpty(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
