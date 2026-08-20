// Package broker owns the durable Kafka-compatible publish boundary.
package broker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"github.com/segmentio/kafka-go"
)

const TelemetryTopic = "sanitized-telemetry-v1"

type KafkaPublisher struct{ writer *kafka.Writer }

func NewKafkaPublisher(brokers []string) *KafkaPublisher {
	return &KafkaPublisher{writer: &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        TelemetryTopic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		Async:        false,
		BatchTimeout: 20 * time.Millisecond,
	}}
}

// Publish returns only after Redpanda acknowledges the sanitized event. No raw
// event is serialized or sent by this adapter.
func (p *KafkaPublisher) Publish(envelope ingest.Envelope) error {
	payload, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(context.Background(), kafka.Message{Key: []byte(envelope.EventKey), Value: payload})
}

func (p *KafkaPublisher) Close() error { return p.writer.Close() }
