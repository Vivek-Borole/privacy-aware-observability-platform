package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/broker"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"github.com/segmentio/kafka-go"
)

type Ledger interface {
	ClaimDelivery(context.Context, string, string) (bool, error)
	MarkPersisted(context.Context, string) error
	RecordLoss(context.Context, string, string) error
}
type Sink interface {
	Persist(context.Context, ingest.Envelope) error
}

type Consumer struct {
	reader *kafka.Reader
	ledger Ledger
	sink   Sink
}

func NewConsumer(brokers []string, ledger Ledger, sink Sink) *Consumer {
	return &Consumer{reader: kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, GroupID: "telemetry-persist-v1", Topic: broker.TelemetryTopic, MinBytes: 1, MaxBytes: 10e6, MaxWait: 500 * time.Millisecond}), ledger: ledger, sink: sink}
}
func (c *Consumer) Close() error { return c.reader.Close() }

// Run commits Kafka only after the ledger says the sanitized event was
// persisted. A malformed message is deliberately left unacknowledged and is
// never logged or copied elsewhere; it cannot become a silent loss.
func (c *Consumer) Run(ctx context.Context) error {
	for {
		message, err := c.reader.FetchMessage(ctx)
		if err != nil {
			return err
		}
		var envelope ingest.Envelope
		if err := json.Unmarshal(message.Value, &envelope); err != nil || envelope.EventKey == "" || envelope.TenantID == "" {
			return errors.New("sanitized broker envelope invalid")
		}
		if err := c.Process(ctx, envelope); err != nil {
			return err
		}
		if err := c.reader.CommitMessages(ctx, message); err != nil {
			return err
		}
	}
}

func (c *Consumer) Process(ctx context.Context, envelope ingest.Envelope) error {
	shouldPersist, err := c.ledger.ClaimDelivery(ctx, envelope.EventKey, envelope.TenantID)
	if err != nil {
		return err
	}
	if !shouldPersist {
		return nil
	}
	if err := c.sink.Persist(ctx, envelope); err != nil {
		return err
	}
	return c.ledger.MarkPersisted(ctx, envelope.EventKey)
}
