# v0.2.0 release notes

This release upgrades the local, synthetic-only v0.1 pipeline without changing
its tenant-scoped query API.

## Added

- official OTLP/gRPC trace and log ingestion on port 4317;
- OTLP/HTTP protobuf ingestion on `/v1/traces` and `/v1/logs`, while retaining
  the bounded JSON contract;
- OpenTelemetry Collector interoperability through the authenticated,
  redaction-first gateway;
- versioned envelopes with current/previous compatibility and a content-free
  dead-letter record for unknown versions;
- atomic 30-second privacy-policy refresh with last-valid fallback;
- Helm and local-kind deployment with two gateway, query, tail, and persistence
  replicas plus rolling-recovery smoke evidence;
- request-latency histograms, v0.2 SLO rules, and an expanded Grafana dashboard.

## Reliability changes

Tail-buffer admission uses deterministic tenant shards to preserve the hard
1,000-active-trace bound without serializing every ingest request. Duplicate
events for an already decided trace remain idempotent. The local benchmark
profile declares its PostgreSQL WAL/checkpoint settings so performance evidence
is reproducible instead of depending on image defaults.

## Limits

This is single-region local evidence on synthetic traffic. Stateful kind
dependencies are single-node. Delivery is at least once with deduplicated
persistence, not globally exactly once. OTLP metrics reception, hosted SaaS,
production customer operation, and multi-region availability are not claimed.
