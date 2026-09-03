# Local benchmark evidence

This directory intentionally excludes raw seeded payloads and local API keys.
Run `bash scripts/run-benchmark.sh` on the documented M3 Pro / 18 GB machine
to create ignored raw JSON and environment artifacts locally. Review the raw
counts, failure conditions, machine data, Compose resource state, and sampled
trace lookup report before creating a signed release report. The checked-in
release summary contains only measured aggregates and commands.

It also receives the synthetic-only screenshots and recorded local demo listed
in [the demo script](../demo-script.md). Do not add those artifacts until the
release gate has passed and a manual raw-seed review confirms they contain no
credential, PII, or unredacted telemetry.

The v0.2 benchmark passes only when the native-OTLP run lasts 10 minutes,
actually accepts at least 5,000 spans/second, accepts every emitted span, and
`lookupPass` is true (`p95Millis <= 3000`).
The harness calls `scripts/validate-benchmark-report.mjs`, which rejects a
short run, failed requests, less than 5,000 accepted spans/second, or a slow
lookup result.

Failed exploratory results may remain locally with a `failed-*` filename to
make tuning honest, but they are not release evidence and are ignored by Git.
The reviewed v0.2 aggregates belong in `v0.2.0-release-verification.md`; raw
reports and environment captures are attached to the release after a privacy
scan rather than silently substituted into source documentation.
