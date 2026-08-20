# Release gates

The repository remains private until all of these are measured and published:

- redaction, tenant isolation, ingestion, query, retention, restart, and [backpressure](backpressure-test.md) tests;
- CI type checks, race tests, dependency audit, and a payload-silent
  high-confidence secret-signature scan;
- 5,000 synthetic spans/s for 10 minutes on the M3 Pro development machine;
- p95 sanitized trace lookup at or below three seconds for that dataset;
- evidence that accepted data survives planned consumer restarts, or loss is explicitly evidenced;
- proof seeded secrets and PII are absent from the PostgreSQL tail buffer/outbox, Redpanda, ClickHouse, logs, exported traces, screenshots, and release artifacts. `scripts/integration-smoke.sh` performs the durable-store checks, including a payload-silent Redpanda audit;
- Compose quick start, threat/privacy model, benchmark and failure reports, synthetic screenshots, and a recorded local demo.

`cmd/synthetic-emitter` is the only load source used for the benchmark. It
emits fabricated OTLP/HTTP JSON spans containing HTTP, queue, database, and
application attributes, deliberately including a synthetic email so the
redaction boundary is exercised. It never connects to real services or uses
production credentials.
