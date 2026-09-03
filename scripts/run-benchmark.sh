#!/usr/bin/env bash
# Runs the documented synthetic benchmark and writes local-only raw evidence.
set -euo pipefail

if docker compose version >/dev/null 2>&1; then compose=(docker compose); else compose=(docker-compose); fi

evidence_dir='docs/evidence'
mkdir -p "$evidence_dir"
api_key="${PAOP_BENCHMARK_KEY:-$(node -e 'console.log(require("crypto").randomBytes(24).toString("hex"))')}"
postgres_url='postgres://paop:paop-local-only@127.0.0.1:5433/paop?sslmode=disable'
tenant_id="synthetic-benchmark-$(date +%s%N)"
raw="$evidence_dir/ingestion-benchmark-v0.2.raw.json"
lookup="$evidence_dir/lookup-benchmark-v0.2.raw.json"
environment="$evidence_dir/benchmark-environment-v0.2.txt"

# Preserve one percent of healthy traces during the benchmark. The normal local
# demo defaults to one so its small synthetic trace is deterministic, but that
# would make the high-volume run measure all-retained behavior rather than the
# platform's bounded tail-sampling design.
export PAOP_HEALTHY_SAMPLE_MODULO=100
compose_up=("${compose[@]}" up -d)
if [[ "${PAOP_SKIP_BUILD:-0}" != '1' ]]; then compose_up+=(--build); fi
compose_up+=(--scale tailer=2 --scale persist=2 postgres redpanda clickhouse migrate topic-init clickhouse-migrate tailer persist gateway query)
"${compose_up[@]}"
PAOP_POSTGRES_URL="$postgres_url" PAOP_TENANT_ID="$tenant_id" PAOP_API_KEY="$api_key" go run ./cmd/bootstrap >/dev/null

{
  echo "startedAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  sw_vers 2>/dev/null || uname -a
  system_profiler SPHardwareDataType 2>/dev/null | sed -n '/Model Name:/p;/Model Identifier:/p;/Chip:/p;/Total Number of Cores:/p;/Memory:/p' || true
  go version
  node --version
  docker version --format '{{.Server.Version}}'
  docker info --format 'dockerCPUs={{.NCPU}} dockerMemoryBytes={{.MemTotal}} storageDriver={{.Driver}}'
  "${compose[@]}" version
  "${compose[@]}" ps
} > "$environment"

# Drive a 2% margin so wall-clock startup/shutdown overhead cannot make a
# healthy 5,000 spans/s run appear below the acceptance threshold.
go run ./cmd/synthetic-emitter \
  -endpoint http://127.0.0.1:18080/v1/traces \
  -transport protobuf \
  -api-key "$api_key" -rate 5100 -workers 48 -batch-size 200 -duration 10m \
  -trace-sample-limit 100 -healthy-sample-modulo "$PAOP_HEALTHY_SAMPLE_MODULO" -output "$raw"

{
  echo
  echo "postRunContainerResources:"
  docker stats --no-stream --format '{{.Name}} cpu={{.CPUPerc}} memory={{.MemUsage}}'
} >> "$environment"

# Query the returned, synthetic-only trace sample set. The key exists only in
# this process environment and must not be copied into an evidence artifact.
PAOP_BENCHMARK_KEY="$api_key" node scripts/lookup-benchmark.mjs "$raw" "$lookup"

node scripts/validate-benchmark-report.mjs "$raw" "$lookup"

echo "benchmark raw artifacts written under $evidence_dir; review them before making any performance claim"
