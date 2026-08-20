# Compose quick start

Prerequisites: current Docker Desktop or Colima, Go 1.26+, Node 22+, and pnpm
11+. All examples use fabricated data.

```bash
pnpm install
go test -race ./...
bash scripts/integration-smoke.sh
```

The smoke command starts PostgreSQL, Redpanda, ClickHouse, migrations, gateway,
persistence worker, tail-sampling worker, and query API. It creates a local
synthetic tenant, sends a seeded OTLP/HTTP trace and a trace-linked OTLP log,
waits for persistence, verifies redaction, explicit sampling evidence, and
tenant-scoped lookup, then rejects any raw seed in PostgreSQL, Redpanda,
ClickHouse, service logs, and metrics. The Redpanda assertion uses an in-memory
audit command that reports only a message count, never broker payloads. It also
proves malformed OTLP/HTTP input is rejected before it reaches the durable
tail buffer.

For the investigation console:

```bash
pnpm console:dev
```

Open the Vite address, enter `http://localhost:18081`, a local tenant API key,
and a trace ID. The API key is sent only as the request header and is not stored
by the console. The **24-hour overview** uses the same authenticated tenant
context and returns derived span, trace, log, and error-marked-span counts;
it does not return raw telemetry attributes.

Prometheus is available at `http://localhost:19090` and the provisioned local
Grafana dashboard at `http://localhost:13000`. The OpenTelemetry Collector
scrapes and exports bounded gateway/query HTTP outcomes to Prometheus; no
tenant IDs, trace IDs, attributes, keys, or telemetry content become metric
labels. The collector is deliberately not a raw tenant-telemetry ingest path.

The synthetic smoke test verifies that the collector exposes the internal
`paop_http_requests_total` metric and rejects unsafe content in those metric
labels.

## Synthetic distributed demo

With the Compose stack running, send one fabricated checkout request:

```bash
curl -X POST http://127.0.0.1:18090/checkout
```

It returns a synthetic trace ID. Query that ID with the console using the local
demo key `synthetic-compose-key-not-for-production`. The trace contains the
TypeScript gateway, Go downstream service, and asynchronous worker. Its seeded
email is intentionally redacted before Redpanda and ClickHouse persistence.
The downstream and worker spans retain only their technical parent span IDs, so
the trace timeline can show causal ordering without storing request content.
