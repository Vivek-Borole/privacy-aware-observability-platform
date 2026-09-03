# Backpressure integration test

Run the fabricated-data pressure test with Docker:

```bash
bash scripts/backpressure-smoke.sh
```

The test stops the tailer, stages 1,001 distinct synthetic traces, and checks
that the PostgreSQL tail buffer never exceeds 1,000 active traces. Every trace
outside the active buffer must produce an `evicted_pressure` decision with
`retained: false`, and the buffer-row count must equal the active count. Ten
deterministic lock shards trade perfect capacity utilization for concurrent
admission, so hash skew can produce evidenced eviction before the theoretical
global ceiling. This is intentional, evidenced sampling loss—not an unrecorded
delivery failure. The test also checks that its raw API key is not present in
the PostgreSQL dump.
