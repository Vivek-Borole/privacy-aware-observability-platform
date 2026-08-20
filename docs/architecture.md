# Architecture

```mermaid
flowchart LR
  A["Synthetic TypeScript gateway"] --> G["Go ingestion gateway"]
  B["Go downstream service"] --> G
  C["Async worker"] --> G
  G -->|"authenticate + validate"| R["redaction policy"]
  R -->|"sanitized envelope + receipt"| K["Redpanda"]
  K --> W["Go persistence worker"]
  W --> H["ClickHouse telemetry"]
  W --> P["PostgreSQL metadata"]
  H --> Q["tenant-scoped query API"]
  P --> Q
  Q --> U["React investigation console"]
```

The gateway accepts bounded OTLP/HTTP JSON traces and logs. It acknowledges an
event only after its sanitized envelope is durably written to the broker.
Redaction happens before that boundary. The persistence worker deduplicates
event identity before ClickHouse storage. Trace-linked logs retain only their
technical trace/span IDs; unlinked logs remain tenant-scoped but are not shown
in a trace lookup. A downstream failure therefore yields retry or an explicit
loss receipt rather than silent success.

Raw telemetry is untrusted input and never reaches broker, storage, or logs before validation and redaction. PostgreSQL metadata and ClickHouse queries always include tenant scope.
