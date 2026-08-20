// Package tail converts durable sanitized trace buffers into explicit sampling
// decisions and a broker outbox. PostgreSQL is the authority; process memory is
// used only for one claimed batch and may be lost safely on restart.
package tail

import (
	"context"
	"fmt"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/metadata"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/sampling"
)

type Publisher interface{ Publish(ingest.Envelope) error }

type Store interface {
	ClaimTailTraces(context.Context, string, time.Duration, time.Duration, int) ([]metadata.TailTraceLease, error)
	LoadTailTrace(context.Context, metadata.TailTraceLease) ([]ingest.Envelope, error)
	RecordTailDecision(context.Context, string, metadata.TailTraceLease, sampling.Decision) (bool, error)
	ClaimTailOutbox(context.Context, string, time.Duration, int) ([]metadata.TailOutboxMessage, error)
	MarkTailOutboxPublished(context.Context, string, string) error
}

type Config struct {
	Owner, Interval string
	Quiet, LeaseTTL time.Duration
	BatchSize       int
	Sampling        sampling.Config
}

type Runner struct {
	Store     Store
	Publisher Publisher
	Config    Config
}

func (r Runner) Run(ctx context.Context) error {
	if r.Config.Interval == "" {
		r.Config.Interval = "1s"
	}
	interval, err := time.ParseDuration(r.Config.Interval)
	if err != nil {
		return err
	}
	if err := r.Process(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := r.Process(ctx); err != nil {
				return err
			}
		}
	}
}

// Process is intentionally small and restart-safe. Published outbox records
// are marked only after broker acknowledgement; an interruption in between
// produces a duplicate broker message that downstream delivery deduplicates by
// its stable event key.
func (r Runner) Process(ctx context.Context) error {
	config := r.Config
	if config.Owner == "" {
		config.Owner = "tailer"
	}
	if config.Quiet <= 0 {
		config.Quiet = 2 * time.Second
	}
	if config.LeaseTTL <= 0 {
		config.LeaseTTL = 15 * time.Second
	}
	if config.BatchSize < 1 {
		config.BatchSize = 32
	}
	leases, err := r.Store.ClaimTailTraces(ctx, config.Owner, config.Quiet, config.LeaseTTL, config.BatchSize)
	if err != nil {
		return fmt.Errorf("claim_tail_traces: %w", err)
	}
	for _, lease := range leases {
		spans, err := r.Store.LoadTailTrace(ctx, lease)
		if err != nil {
			return fmt.Errorf("load_tail_trace: %w", err)
		}
		decision := sampling.Decide(lease.TraceID, spans, config.Sampling)
		if _, err := r.Store.RecordTailDecision(ctx, config.Owner, lease, decision); err != nil {
			return fmt.Errorf("record_tail_decision: %w", err)
		}
	}
	messages, err := r.Store.ClaimTailOutbox(ctx, config.Owner, config.LeaseTTL, config.BatchSize)
	if err != nil {
		return fmt.Errorf("claim_tail_outbox: %w", err)
	}
	for _, message := range messages {
		if err := r.Publisher.Publish(message.Envelope); err != nil {
			return fmt.Errorf("publish_tail_outbox: %w", err)
		}
		if err := r.Store.MarkTailOutboxPublished(ctx, config.Owner, message.EventKey); err != nil {
			return fmt.Errorf("mark_tail_outbox: %w", err)
		}
	}
	return nil
}
