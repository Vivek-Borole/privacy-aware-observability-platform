#!/usr/bin/env bash
# Verifies durable tail-buffer pressure is explicit and bounded.
set -euo pipefail

if docker compose version >/dev/null 2>&1; then compose=(docker compose); else compose=(docker-compose); fi

postgres_url='postgres://paop:paop-local-only@127.0.0.1:5433/paop?sslmode=disable'
run_suffix="$(date +%s%N)"
tenant_id="synthetic-pressure-$run_suffix"
api_key="pressure-key-$run_suffix-not-for-production"

compose_up=("${compose[@]}" up -d)
if [[ "${PAOP_SKIP_BUILD:-0}" != '1' ]]; then compose_up+=(--build); fi
"${compose_up[@]}" postgres redpanda clickhouse migrate topic-init clickhouse-migrate tailer persist gateway query
PAOP_POSTGRES_URL="$postgres_url" PAOP_TENANT_ID="$tenant_id" PAOP_API_KEY="$api_key" go run ./cmd/bootstrap >/dev/null

# The tailer must not consume staged buffers while this test drives the fixed
# v1 pressure bound of 1,000 active traces.
"${compose[@]}" stop tailer
PAOP_POSTGRES_URL="$postgres_url" go run ./cmd/tail-pressure -tenant "$tenant_id" -count 1001 >/dev/null

active=$("${compose[@]}" exec -T postgres psql -U paop -d paop -At -c "select count(*) from tail_traces where tenant_id = '$tenant_id' and decided_at is null")
evicted=$("${compose[@]}" exec -T postgres psql -U paop -d paop -At -c "select count(*) from tail_decisions where tenant_id = '$tenant_id' and reason = 'evicted_pressure' and retained = false")
buffers=$("${compose[@]}" exec -T postgres psql -U paop -d paop -At -c "select count(*) from tail_buffers where tenant_id = '$tenant_id'")
test "$active" = '1000'
test "$buffers" = '1000'
test "$evicted" = '1'

metadata_dump=$("${compose[@]}" exec -T postgres pg_dump -U paop paop)
if [[ "$metadata_dump" == *"$api_key"* ]]; then
  echo 'raw pressure-test API key appeared in PostgreSQL' >&2
  exit 1
fi
echo 'backpressure smoke passed: the tail buffer stayed at 1,000 active traces and recorded one explicit pressure eviction'
