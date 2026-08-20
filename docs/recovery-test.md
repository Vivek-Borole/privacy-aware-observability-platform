# Recovery integration test

Run the fault-injection smoke test with Docker:

```bash
bash scripts/recovery-smoke.sh
```

The test uses fabricated OTLP/HTTP telemetry only. It stops the tailer before
the gateway accepts a span, proving the gateway's durable PostgreSQL staging
boundary; it then restarts the tailer while ClickHouse is paused, resumes
storage, and restarts the persistence worker. It proves the span is eventually
visible once, and that sending the same event identity again does not create a
second visible query effect. It also checks seeded secret, email, and API-key
absence from query output and service logs.

This is a bounded recovery scenario—not proof of every broker, disk, network,
or multi-node failure mode. The public-release gate still requires the broader
failure report and measured benchmark evidence.
