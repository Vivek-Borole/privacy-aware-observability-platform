package telemetry

import (
	"context"
	"crypto/sha256"
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
type DeadLetterRecorder interface {
	RecordDeadLetter(context.Context, string, string, int, string, string, int, int64) error
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
			if err := c.deadLetter(ctx, message, envelope, "malformed_envelope"); err != nil {
				return err
			}
			if err := c.reader.CommitMessages(ctx, message); err != nil {
				return err
			}
			continue
		}
		normalizedVersion, supported := supportedEnvelopeVersion(envelope.SchemaVersion)
		envelope.SchemaVersion = normalizedVersion
		if !supported {
			if err := c.deadLetter(ctx, message, envelope, "unknown_schema_version"); err != nil {
				return err
			}
			if err := c.reader.CommitMessages(ctx, message); err != nil {
				return err
			}
			continue
		}
		if err := c.Process(ctx, envelope); err != nil {
			return err
		}
		if err := c.reader.CommitMessages(ctx, message); err != nil {
			return err
		}
	}
}

func supportedEnvelopeVersion(version int) (int, bool) {
	if version == 0 {
		version = ingest.PreviousEnvelopeVersion
	}
	return version, version == ingest.PreviousEnvelopeVersion || version == ingest.CurrentEnvelopeVersion
}

func (c *Consumer) deadLetter(ctx context.Context, message kafka.Message, envelope ingest.Envelope, reason string) error {
	recorder, ok := c.ledger.(DeadLetterRecorder)
	if !ok {
		return errors.New("dead-letter recorder unavailable")
	}
	digest := sha256.Sum256(message.Value)
	return recorder.RecordDeadLetter(ctx, envelope.TenantID, envelope.EventKey, envelope.SchemaVersion, reason, fmtHex(digest[:]), message.Partition, message.Offset)
}

func fmtHex(value []byte) string {
	const alphabet = "0123456789abcdef"
	result := make([]byte, len(value)*2)
	for index, byteValue := range value {
		result[index*2] = alphabet[byteValue>>4]
		result[index*2+1] = alphabet[byteValue&0x0f]
	}
	return string(result)
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
