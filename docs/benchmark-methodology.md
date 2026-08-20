# Benchmark methodology

The release benchmark is run only on the stated M3 Pro / 18 GB development
machine after a clean Compose start. It uses no production data. Run the
reproducible harness:

```bash
bash scripts/run-benchmark.sh
```

The harness records start time, operating system/architecture, CPU count,
target rate, actual elapsed time, actual accepted throughput, request totals,
accepted/failure counts, p50/p95/p99 ingest latency, a bounded list of
successful synthetic trace IDs, Docker/Go/Node versions, Compose state, and
sampled ClickHouse lookup p50/p95/p99. The release report must also record
observed service resource use and any failure conditions. The machine-readable
validator requires a full ten-minute run, zero failures, at least 5,000 actual
accepted spans/second, and sanitized trace lookup p95 at most three seconds.

The command writes a result only after the actual run. No benchmark result is
committed or claimed before that local workload finishes. Validate an existing
synthetic report with:

```bash
node scripts/validate-benchmark-report.mjs docs/evidence/ingestion-benchmark.raw.json docs/evidence/lookup-benchmark.raw.json
```
