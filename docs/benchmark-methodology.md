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
validator requires a full ten-minute run, zero failures, a load target of at
least 5,000 spans/second, at least 5,000 actual accepted spans/second, and
sanitized trace lookup p95 at most three seconds. The default harness drives
5,100 spans/second to make the minimum robust against end-of-run flush timing;
its raw report records both numbers.

The regular Compose demo retains every healthy trace (`PAOP_HEALTHY_SAMPLE_MODULO=1`)
so a small fabricated scenario is predictable. The benchmark overrides it to
`100`, retaining deterministic one-percent healthy samples while still
measuring every accepted ingest request. Its stored lookup sample contains only
trace IDs selected by that same FNV-1a decision rule.

The emitter sends bounded OTLP batches of 100 synthetic spans. Throughput is
reported in spans per second; request-latency percentiles describe those batch
requests. This avoids treating host timer granularity or per-request overhead
as a span-throughput result while preserving transactional batch validation.

The command writes a result only after the actual run. No benchmark result is
committed or claimed before that local workload finishes. Validate an existing
synthetic report with:

```bash
node scripts/validate-benchmark-report.mjs docs/evidence/ingestion-benchmark.raw.json docs/evidence/lookup-benchmark.raw.json
```
