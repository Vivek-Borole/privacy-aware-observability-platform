package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/sampling"
)

const (
	defaultTailMaxTraces = 1000
	defaultTailMaxSpans  = 100
)

var (
	ErrTailBufferFull  = errors.New("tail buffer pressure")
	ErrTailTraceClosed = errors.New("tail trace already decided")
)

// TailTraceLease identifies one durable trace buffer held by a tail worker.
// Generation prevents a worker from deciding a trace to which a new span was
// added after it acquired the lease.
type TailTraceLease struct {
	TenantID   string
	TraceID    string
	Generation int64
}

// TailDecision is safe investigation metadata: it names the sampling outcome
// and cardinality, never telemetry content.
type TailDecision struct {
	TraceID   string    `json:"traceId"`
	Reason    string    `json:"reason"`
	Retained  bool      `json:"retained"`
	SpanCount int       `json:"spanCount"`
	CreatedAt time.Time `json:"createdAt"`
}

// TailOutboxMessage is a sanitized envelope awaiting broker acknowledgement.
// The source envelope has already crossed the PostgreSQL durability boundary.
type TailOutboxMessage struct {
	EventKey string
	Envelope ingest.Envelope
}

// Stage durably buffers an already-sanitized envelope before gateway success.
// Pressure and span-limit evictions are explicit durable decisions, not silent
// drops. The fixed v1 bounds prevent one tenant from consuming unbounded local
// memory; the buffer itself is PostgreSQL-backed and survives worker restarts.
func (s *Store) Stage(ctx context.Context, envelope ingest.Envelope) error {
	traceID := tailTraceID(envelope)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `select pg_advisory_xact_lock(hashtext($1))`, envelope.TenantID); err != nil {
		return err
	}
	var spanCount int
	var decidedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `select span_count, decided_at from tail_traces where tenant_id = $1 and trace_id = $2 for update`, envelope.TenantID, traceID).Scan(&spanCount, &decidedAt)
	if errors.Is(err, sql.ErrNoRows) {
		var activeTraces int
		if err := tx.QueryRowContext(ctx, `select count(*) from tail_traces where tenant_id = $1 and decided_at is null`, envelope.TenantID).Scan(&activeTraces); err != nil {
			return err
		}
		if activeTraces >= defaultTailMaxTraces {
			if err := s.evictOldestTailTrace(ctx, tx, envelope.TenantID, "evicted_pressure"); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `insert into tail_traces(tenant_id, trace_id) values ($1, $2)`, envelope.TenantID, traceID); err != nil {
			return err
		}
		spanCount = 0
	} else if err != nil {
		return err
	} else if decidedAt.Valid {
		return ErrTailTraceClosed
	}

	payload, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `insert into tail_buffers(event_key, tenant_id, trace_id, envelope) values ($1, $2, $3, $4::jsonb) on conflict (event_key) do nothing`, envelope.EventKey, envelope.TenantID, traceID, string(payload))
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 0 {
		return tx.Commit()
	}
	if spanCount >= defaultTailMaxSpans {
		if err := s.evictTailTrace(ctx, tx, envelope.TenantID, traceID, "evicted_span_limit"); err != nil {
			return err
		}
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `update tail_traces set last_seen = now(), generation = generation + 1, span_count = span_count + 1 where tenant_id = $1 and trace_id = $2 and decided_at is null`, envelope.TenantID, traceID); err != nil {
		return err
	}
	return tx.Commit()
}

func tailTraceID(envelope ingest.Envelope) string {
	if envelope.Event.TraceID != "" {
		return envelope.Event.TraceID
	}
	return "unlinked-log:" + envelope.Event.EventID
}

func (s *Store) evictOldestTailTrace(ctx context.Context, tx *sql.Tx, tenantID, reason string) error {
	var traceID string
	err := tx.QueryRowContext(ctx, `select trace_id from tail_traces where tenant_id = $1 and decided_at is null order by first_seen asc for update skip locked limit 1`, tenantID).Scan(&traceID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTailBufferFull
	}
	if err != nil {
		return err
	}
	return s.evictTailTrace(ctx, tx, tenantID, traceID, reason)
}

func (s *Store) evictTailTrace(ctx context.Context, tx *sql.Tx, tenantID, traceID, reason string) error {
	var count int
	if err := tx.QueryRowContext(ctx, `select count(*) from tail_buffers where tenant_id = $1 and trace_id = $2`, tenantID, traceID).Scan(&count); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from tail_buffers where tenant_id = $1 and trace_id = $2`, tenantID, traceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update tail_traces set decision = $3, decided_at = now(), lease_owner = null, lease_expires_at = null, span_count = $4 where tenant_id = $1 and trace_id = $2 and decided_at is null`, tenantID, traceID, reason, count); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `insert into tail_decisions(tenant_id, trace_id, reason, retained, span_count) values ($1, $2, $3, false, $4) on conflict (tenant_id, trace_id) do nothing`, tenantID, traceID, reason, count)
	return err
}

// ClaimTailTraces leases quiet, undecided traces. A later write changes the
// generation and makes the worker's later decision a safe no-op.
func (s *Store) ClaimTailTraces(ctx context.Context, owner string, quiet, ttl time.Duration, limit int) ([]TailTraceLease, error) {
	if limit < 1 {
		limit = 1
	}
	rows, err := s.db.QueryContext(ctx, `
with selected as (
  select tenant_id, trace_id from tail_traces
  where decided_at is null and last_seen <= now() - ($1 * interval '1 microsecond')
    and (lease_expires_at is null or lease_expires_at < now())
  order by last_seen asc
  for update skip locked
  limit $2
)
update tail_traces traces set lease_owner = $3, lease_expires_at = now() + ($4 * interval '1 microsecond')
from selected where traces.tenant_id = selected.tenant_id and traces.trace_id = selected.trace_id
returning traces.tenant_id, traces.trace_id, traces.generation`, quiet.Microseconds(), limit, owner, ttl.Microseconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var leases []TailTraceLease
	for rows.Next() {
		var lease TailTraceLease
		if err := rows.Scan(&lease.TenantID, &lease.TraceID, &lease.Generation); err != nil {
			return nil, err
		}
		leases = append(leases, lease)
	}
	return leases, rows.Err()
}

func (s *Store) LoadTailTrace(ctx context.Context, lease TailTraceLease) ([]ingest.Envelope, error) {
	rows, err := s.db.QueryContext(ctx, `select envelope from tail_buffers where tenant_id = $1 and trace_id = $2 order by received_at asc`, lease.TenantID, lease.TraceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var envelopes []ingest.Envelope
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var envelope ingest.Envelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			return nil, err
		}
		envelopes = append(envelopes, envelope)
	}
	return envelopes, rows.Err()
}

// RecordTailDecision copies retained, sanitized envelopes to a durable outbox
// atomically with the decision. If the trace changed after leasing, it returns
// false and leaves the trace for a later worker instead of deciding stale data.
func (s *Store) RecordTailDecision(ctx context.Context, owner string, lease TailTraceLease, decision sampling.Decision) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var generation int64
	var decidedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `select generation, decided_at from tail_traces where tenant_id = $1 and trace_id = $2 and lease_owner = $3 for update`, lease.TenantID, lease.TraceID, owner).Scan(&generation, &decidedAt)
	if errors.Is(err, sql.ErrNoRows) || decidedAt.Valid || generation != lease.Generation {
		return false, tx.Commit()
	}
	if err != nil {
		return false, err
	}
	var spanCount int
	if err := tx.QueryRowContext(ctx, `select count(*) from tail_buffers where tenant_id = $1 and trace_id = $2`, lease.TenantID, lease.TraceID).Scan(&spanCount); err != nil {
		return false, err
	}
	if decision.Retained {
		rows, err := tx.QueryContext(ctx, `select event_key, envelope from tail_buffers where tenant_id = $1 and trace_id = $2 order by received_at asc`, lease.TenantID, lease.TraceID)
		if err != nil {
			return false, err
		}
		for rows.Next() {
			var eventKey string
			var payload []byte
			if err := rows.Scan(&eventKey, &payload); err != nil {
				rows.Close()
				return false, err
			}
			if _, err := tx.ExecContext(ctx, `insert into tail_outbox(event_key, tenant_id, envelope) values ($1, $2, $3::jsonb) on conflict (event_key) do nothing`, eventKey, lease.TenantID, string(payload)); err != nil {
				rows.Close()
				return false, err
			}
		}
		if err := rows.Close(); err != nil {
			return false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `delete from tail_buffers where tenant_id = $1 and trace_id = $2`, lease.TenantID, lease.TraceID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `update tail_traces set decision = $4, decided_at = now(), lease_owner = null, lease_expires_at = null, span_count = $5 where tenant_id = $1 and trace_id = $2 and lease_owner = $3`, lease.TenantID, lease.TraceID, owner, decision.Reason, spanCount); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `insert into tail_decisions(tenant_id, trace_id, reason, retained, span_count) values ($1, $2, $3, $4, $5)`, lease.TenantID, lease.TraceID, decision.Reason, decision.Retained, spanCount); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) ClaimTailOutbox(ctx context.Context, owner string, ttl time.Duration, limit int) ([]TailOutboxMessage, error) {
	if limit < 1 {
		limit = 1
	}
	rows, err := s.db.QueryContext(ctx, `
with selected as (
  select event_key from tail_outbox
  where published_at is null and (lease_expires_at is null or lease_expires_at < now())
  order by created_at asc
  for update skip locked
  limit $1
)
update tail_outbox outbox set lease_owner = $2, lease_expires_at = now() + ($3 * interval '1 microsecond'), attempts = attempts + 1
from selected where outbox.event_key = selected.event_key
returning outbox.event_key, outbox.envelope`, limit, owner, ttl.Microseconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []TailOutboxMessage
	for rows.Next() {
		var message TailOutboxMessage
		var payload []byte
		if err := rows.Scan(&message.EventKey, &payload); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &message.Envelope); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (s *Store) MarkTailOutboxPublished(ctx context.Context, owner, eventKey string) error {
	result, err := s.db.ExecContext(ctx, `update tail_outbox set published_at = now(), lease_owner = null, lease_expires_at = null where event_key = $1 and lease_owner = $2 and published_at is null`, eventKey, owner)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return errors.New("tail outbox lease lost")
	}
	return nil
}

func (s *Store) TailDecisions(ctx context.Context, tenantID string, limit int) ([]TailDecision, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `select trace_id, reason, retained, span_count, created_at from tail_decisions where tenant_id = $1 order by id desc limit $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var decisions []TailDecision
	for rows.Next() {
		var decision TailDecision
		if err := rows.Scan(&decision.TraceID, &decision.Reason, &decision.Retained, &decision.SpanCount, &decision.CreatedAt); err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, rows.Err()
}
