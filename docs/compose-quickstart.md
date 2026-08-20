# Compose quick start

Prerequisites: current Docker Desktop or Colima, Go 1.26+, Node 22+, and pnpm
11+. All examples use fabricated data.

```bash
pnpm install
go test -race ./...
bash scripts/integration-smoke.sh
```

The smoke command starts PostgreSQL, Redpanda, ClickHouse, migrations, gateway,
persistence worker, and query API. It creates a local synthetic tenant, sends a
seeded OTLP/HTTP trace, waits for persistence, verifies redaction and
tenant-scoped lookup, then rejects any raw seed in service logs.

For the investigation console:

```bash
pnpm console:dev
```

Open the Vite address, enter `http://localhost:18081`, a local tenant API key,
and a trace ID. The API key is sent only as the request header and is not stored
by the console. The **24-hour overview** uses the same authenticated tenant
context and returns derived span, trace, and error-marked-span counts only; it
does not return raw telemetry attributes.

## Synthetic distributed demo

With the Compose stack running, send one fabricated checkout request:

```bash
curl -X POST http://127.0.0.1:18090/checkout
```

It returns a synthetic trace ID. Query that ID with the console using the local
demo key `synthetic-compose-key-not-for-production`. The trace contains the
TypeScript gateway, Go downstream service, and asynchronous worker. Its seeded
email is intentionally redacted before Redpanda and ClickHouse persistence.
