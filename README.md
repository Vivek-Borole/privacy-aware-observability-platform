# Privacy-Aware Observability Platform

A local-first, multi-tenant telemetry pipeline and incident-investigation console. This repository is private during development and will be published only after its privacy, reliability, benchmark, and evidence gates pass.

## Scope

- A synthetic TypeScript gateway, Go downstream service, and asynchronous worker emit trace-shaped events.
- A Go gateway authenticates tenants, validates bounded input, redacts sensitive attributes before durable publication, and records policy-versioned redaction receipts.
- Redpanda provides durable streaming, ClickHouse stores sanitized telemetry, and PostgreSQL holds tenant, API-key, retention, and policy metadata.
- A future React console will show tenant-scoped sanitized traces, async causal links, service dependencies, usage, and incident timelines.

## Non-goals

- Not a production SaaS, replay engine, or raw-customer-secret store.
- Never persist authorization headers, cookies, API keys, or plain email addresses from accepted telemetry.
- Never claim silent losslessness: accepted events are durably acknowledged or an explicit evidenced loss state is reported.

## Delivery sequence

1. Foundation, data-flow contract, and redaction core.
2. Synthetic services plus authenticated, durable OTLP-shaped ingestion.
3. ClickHouse query path and React investigation console.
4. Failure injection, 5,000 spans/s benchmark, evidence package, and public tagged release.

Read [the architecture](docs/architecture.md), [privacy model](docs/privacy-model.md), and [release gates](docs/release-gates.md) before extending the platform.

## Local foundation check

```bash
go test -race ./...
# Docker Desktop commonly provides `docker compose`; this Mac's Colima setup uses `docker-compose`.
docker compose config || docker-compose config
```

Run the complete synthetic integration check (Docker required):

```bash
bash scripts/integration-smoke.sh
```

It bootstraps a local synthetic tenant, submits an OTLP/HTTP trace with seeded
secret/PII fields, waits for the authorized query result, and fails if the raw
values appear in query output or service logs.
