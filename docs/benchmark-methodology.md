# Benchmark methodology

The release benchmark is run only on the stated M3 Pro / 18 GB development
machine after a clean Compose start. It uses no production data. Run the
reproducible harness:

```bash
bash scripts/run-benchmark.sh
```

The harness records start time, operating system/architecture, CPU count,
target rate, request totals, accepted/failure counts, p50/p95/p99 ingest
latency, a bounded list of successful synthetic trace IDs, Docker/Go/Node
versions, Compose state, and sampled ClickHouse lookup p50/p95/p99. The release
report must also record observed service resource use and any failure conditions.
It passes only if the stated workload completes without unexplained loss and
the separately measured sanitized trace lookup p95 is at most three seconds.

The command writes a result only after the actual run. No benchmark result is
committed or claimed before that local workload finishes.
