# Threat model

## Assets

- tenant telemetry after redaction;
- tenant API-key hashes and policy/retention metadata;
- delivery-ledger state and deletion audit records;
- local-only synthetic benchmark artifacts.

Raw authorization headers, cookies, bearer/API keys, emails, prompts, and
customer content are explicitly not assets the platform is allowed to retain.

## Boundaries and mitigations

| Boundary | Threat | Mitigation |
| --- | --- | --- |
| Browser → query API | Cross-tenant lookup or key exfiltration | Authentication resolves the tenant from a hashed key; ClickHouse queries bind that tenant; CORS permits only the configured console origin. |
| Producer → gateway | PII/secrets reach durable systems | Size/schema limits and versioned deployment/tenant redaction happen before PostgreSQL staging or Redpanda publication. |
| Redpanda → persistence worker | Duplicate delivery or worker restart | A PostgreSQL delivery ledger controls identity; ClickHouse uses deterministic event keys and `FINAL` queries. |
| Persistence → ClickHouse | Unsafe raw values or SQL injection | Only sanitized envelopes are serialized; tenant/trace/retention values use ClickHouse named parameters. |
| Local operations | Secrets leak into logs/evidence | Services log error classes and identifiers only; the Compose smoke test searches logs and query output for seeded values. |
| Tail buffers | Unbounded memory or invisible loss | Maximum traces/spans, explicit eviction decisions, and error/slow retention rules. |

## Residual risks

The current v1 remains local-first and single-node. A crash after ClickHouse
acceptance but before the ledger update may create a physical duplicate, which
is hidden by `ReplacingMergeTree` plus `FINAL`; it does not claim global
exactly-once execution. A public release remains blocked until restart and
backpressure tests quantify these conditions.
