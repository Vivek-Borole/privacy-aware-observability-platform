# Benchmark methodology

The release benchmark is run only on the stated M3 Pro / 18 GB development
machine after a clean Compose start. It uses no production data.

```bash
go run ./cmd/synthetic-emitter \
  -endpoint http://127.0.0.1:18080/v1/traces \
  -api-key "$PAOP_SYNTHETIC_API_KEY" \
  -rate 5000 -workers 64 -duration 10m \
  -output docs/evidence/ingestion-benchmark.json
```

The command records start time, operating system/architecture, CPU count,
target rate, request totals, accepted/failure counts, and p50/p95/p99 ingest
latency. The release report must also record Docker, Go, and Node versions,
compose service resource use, ClickHouse query p95, and any failure conditions.
It passes only if the stated workload completes without unexplained loss and
the separately measured sanitized trace lookup p95 is at most three seconds.

The command writes a result only after the actual run. No benchmark result is
committed or claimed before that local workload finishes.
