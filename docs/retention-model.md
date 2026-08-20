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
