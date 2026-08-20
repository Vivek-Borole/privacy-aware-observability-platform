# Local benchmark evidence

This directory contains no measured results in source control. Run
`bash scripts/run-benchmark.sh` on the documented M3 Pro / 18 GB machine to
create ignored raw JSON and environment artifacts locally. Review the raw
counts, failure conditions, machine data, Compose resource state, and sampled
trace lookup report before creating a signed release report.

The benchmark passes only when the actual 5,000-spans/second, 10-minute run
has no unexplained failures and `lookupPass` is true (`p95Millis <= 3000`).
The harness calls `scripts/validate-benchmark-report.mjs`, which rejects a
short run, failed requests, less than 5,000 accepted spans/second, or a slow
lookup result.
