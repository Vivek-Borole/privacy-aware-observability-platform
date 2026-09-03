# Privacy-Aware Observability Platform

A local-first, multi-tenant telemetry pipeline and incident-investigation console. The immutable `v0.1.0` release established the baseline; `v0.2.0` adds native OpenTelemetry interoperability and local Kubernetes recovery evidence without claiming production SaaS operation.

## Scope

- A synthetic TypeScript gateway, Go downstream service, and asynchronous worker
  emit one distributed, trace-ID-propagated checkout flow. It is fabricated
  demo traffic only and never reaches an external service.
- A Go gateway authenticates tenants and accepts OTLP/gRPC (`4317`), OTLP/HTTP protobuf (`/v1/traces`, `/v1/logs`), and the bounded legacy JSON format. Every transport is normalized into one envelope, then redacted before durable staging with a policy-versioned receipt.
- Readers support the current and immediately previous envelope version. Unknown versions enter a content-free, evidenced dead-letter path instead of query storage.
- Privacy policies refresh atomically from PostgreSQL every 30 seconds. Invalid updates never replace the last compiled valid policy.
- PostgreSQL holds tenant, API-key, retention, policy, and durable tail-sampling state; Redpanda receives retained telemetry through its retry-safe outbox, and ClickHouse stores the sanitized result.
- The React console shows tenant-scoped sanitized traces (including trace-linked
  logs), a derived 24-hour usage/error summary, service dependency map, and
  safe audit timeline. These are local development capabilities, not a public
  SaaS or release-readiness claim.

## Non-goals

- Not a production SaaS, replay engine, or raw-customer-secret store.
- Never persist authorization headers, cookies, API keys, or plain email addresses from accepted telemetry.
- Never claim silent losslessness: accepted events are durably acknowledged or an explicit evidenced loss state is reported.

## Delivery sequence

1. Foundation, data-flow contract, and redaction core.
2. Synthetic services plus authenticated, durable OTLP-shaped ingestion.
3. ClickHouse query path and React investigation console.
4. Native OTLP, two-replica local-kind recovery, policy/schema evolution, failure injection, and the 5,000 spans/s release benchmark.

Read [the architecture](docs/architecture.md), [privacy model](docs/privacy-model.md), [threat model](docs/threat-model.md), [retention model](docs/retention-model.md), [schema and policy evolution](docs/schema-and-policy-evolution.md), [Compose quick start](docs/compose-quickstart.md), [Kubernetes quick start](docs/kubernetes-quickstart.md), and [release gates](docs/release-gates.md) before extending the platform. Contribution, conduct, and private vulnerability-reporting expectations are in [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), and [SECURITY.md](SECURITY.md).

For the Kubernetes path, run `bash scripts/kind-smoke.sh`. It builds local images, installs the Helm chart, proves a native protobuf request through two gateway/query replicas, and rolls the stateless and coordinated worker deployments. No cloud account or paid service is used.

The current versioned machine-readable interface is [OpenAPI v1](api/openapi-v1.json). It documents only the implemented local APIs; it is not a public SaaS promise.

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

Run the [recovery integration test](docs/recovery-test.md) to exercise an
accepted event through a temporary storage outage and duplicate delivery.
