# Retention and deletion model

Each tenant has a PostgreSQL retention policy with a default of 30 days. The
`retention` command reads those policies and sends a tenant-bound ClickHouse
mutation for spans older than the computed cutoff:

```bash
PAOP_POSTGRES_URL="$PAOP_POSTGRES_URL" \
PAOP_CLICKHOUSE_URL="$PAOP_CLICKHOUSE_URL" \
go run ./cmd/retention
```

ClickHouse mutations are asynchronous. A successful request means that the
deletion was scheduled, not that bytes were already physically removed. The
future operator console must display this distinction and an audit record for
each tenant/deletion request. Retention affects sanitized telemetry only; raw
secrets are rejected before Redpanda and have no retention path.

The same cutoff removes completed PostgreSQL tail-sampling decisions, event
identity keys, and already-published outbox rows. It intentionally does not
delete an undecided tail buffer or unpublished outbox row: those are still
recoverable accepted telemetry and require investigation rather than a silent
retention deletion.

An authenticated tenant may also request deletion of all of its sanitized
telemetry through `POST /v1/retention/delete`. It requires the exact
`X-PAOP-Delete-Confirm: DELETE_SANITIZED_TELEMETRY` header. The API resolves
the tenant from the API-key hash, immediately removes its PostgreSQL tail
buffer/outbox/decision metadata, sends a tenant-bound ClickHouse mutation, and
records a `tenant_telemetry_deletion_requested` audit receipt. The response
state is `mutation_requested`, never an immediate-deletion promise.
