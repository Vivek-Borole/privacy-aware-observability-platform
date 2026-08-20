#!/usr/bin/env bash
# Runs the documented synthetic benchmark and writes local-only raw evidence.
set -euo pipefail

if docker compose version >/dev/null 2>&1; then compose=(docker compose); else compose=(docker-compose); fi

evidence_dir='docs/evidence'
mkdir -p "$evidence_dir"
api_key="${PAOP_BENCHMARK_KEY:-$(node -e 'console.log(require("crypto").randomBytes(24).toString("hex"))')}"
postgres_url='postgres://paop:paop-local-only@127.0.0.1:5433/paop?sslmode=disable'
raw="$evidence_dir/ingestion-benchmark.raw.json"
lookup="$evidence_dir/lookup-benchmark.raw.json"
environment="$evidence_dir/benchmark-environment.txt"

"${compose[@]}" up -d --build postgres redpanda clickhouse migrate topic-init clickhouse-migrate tailer persist gateway query
PAOP_POSTGRES_URL="$postgres_url" PAOP_TENANT_ID='synthetic-benchmark' PAOP_API_KEY="$api_key" go run ./cmd/bootstrap >/dev/null

{
  echo "startedAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  sw_vers 2>/dev/null || uname -a
  go version
  node --version
  docker version --format '{{.Server.Version}}'
  "${compose[@]}" version
  "${compose[@]}" ps
} > "$environment"

go run ./cmd/synthetic-emitter \
  -endpoint http://127.0.0.1:18080/v1/traces \
  -api-key "$api_key" -rate 5000 -workers 64 -duration 10m \
  -trace-sample-limit 100 -output "$raw"

# Query the returned, synthetic-only trace sample set. The key exists only in
# this process environment and must not be copied into an evidence artifact.
PAOP_BENCHMARK_RAW="$raw" PAOP_BENCHMARK_LOOKUP="$lookup" PAOP_BENCHMARK_KEY="$api_key" node <<'NODE'
const fs = require("fs");
const raw = JSON.parse(fs.readFileSync(process.env.PAOP_BENCHMARK_RAW, "utf8"));
const key = process.env.PAOP_BENCHMARK_KEY;
const base = "http://127.0.0.1:18081";
const latencies = [];
for (const traceId of raw.traceSamples || []) {
  const start = performance.now();
  const response = await fetch(`${base}/v1/traces/${traceId}`, {headers: {"x-paop-api-key": key}});
  if (!response.ok) throw new Error(`lookup failed with ${response.status}`);
  await response.arrayBuffer();
  latencies.push(performance.now() - start);
}
latencies.sort((a, b) => a - b);
const percentile = (p) => latencies.length ? latencies[Math.floor((latencies.length - 1) * p / 100)] : 0;
const report = { sampledTraces: latencies.length, p50Millis: percentile(50), p95Millis: percentile(95), p99Millis: percentile(99), lookupPass: percentile(95) <= 3000 };
fs.writeFileSync(process.env.PAOP_BENCHMARK_LOOKUP, `${JSON.stringify(report, null, 2)}\n`, {mode: 0o600});
console.log(JSON.stringify(report));
NODE

echo "benchmark raw artifacts written under $evidence_dir; review them before making any performance claim"
