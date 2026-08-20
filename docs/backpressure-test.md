# Backpressure integration test

Run the fabricated-data pressure test with Docker:

```bash
bash scripts/backpressure-smoke.sh
```

The test stops the tailer, stages 1,001 distinct synthetic traces, and checks
that the PostgreSQL tail buffer remains bounded at 1,000 active traces. The
oldest trace must produce exactly one `evicted_pressure` decision with
`retained: false`. This is intentional, evidenced sampling loss—not an
unrecorded delivery failure. The test also checks that its raw API key is not
present in the PostgreSQL dump.
