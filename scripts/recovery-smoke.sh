#!/usr/bin/env bash
# Exercises a durably staged event across tailer, storage, and consumer restarts.
set -euo pipefail

if docker compose version >/dev/null 2>&1; then compose=(docker compose); else compose=(docker-compose); fi

api_key='recovery-integration-key-not-for-production'
postgres_url='postgres://paop:paop-local-only@127.0.0.1:5433/paop?sslmode=disable'
gateway_url='http://127.0.0.1:18080/v1/traces'
query_url='http://127.0.0.1:18081/v1/traces/recovery-trace-001'
payload='{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"recovery-synthetic"}},{"key":"authorization","value":{"stringValue":"Bearer recovery-secret-must-not-persist"}}]},"scopeSpans":[{"spans":[{"traceId":"recovery-trace-001","spanId":"recovery-span-001","name":"recovery.checkout","attributes":[{"key":"customer.email","value":{"stringValue":"recovery.user@example.test"}},{"key":"http.status_code","value":{"intValue":"200"}}]}]}]}]}'

"${compose[@]}" up -d --build postgres redpanda clickhouse migrate topic-init clickhouse-migrate tailer persist gateway query
PAOP_POSTGRES_URL="$postgres_url" PAOP_TENANT_ID='synthetic-recovery' PAOP_API_KEY="$api_key" go run ./cmd/bootstrap >/dev/null

# Stop the tailer before acceptance. The successful gateway response must mean
# the sanitized envelope is durable in PostgreSQL, not merely resident in the
# tailer process or already handed to Redpanda.
"${compose[@]}" stop tailer
status=$(curl --silent --show-error --retry 5 --retry-all-errors --retry-delay 1 --output /dev/null --write-out '%{http_code}' --request POST "$gateway_url" --header 'content-type: application/json' --header "x-paop-api-key: $api_key" --data "$payload")
test "$status" = '202'
staged=$("${compose[@]}" exec -T postgres psql -U paop -d paop -At -c "select count(*) from tail_buffers where tenant_id = 'synthetic-recovery' and trace_id = 'recovery-trace-001'")
test "$staged" = '1'

# Recover the tailer while storage is unavailable. It can safely release the
# durable outbox event; the persistence consumer will later redeliver it.
"${compose[@]}" pause clickhouse
"${compose[@]}" start tailer
sleep 6
"${compose[@]}" unpause clickhouse
"${compose[@]}" restart persist

for _ in {1..45}; do
  result=$(curl --silent --show-error "$query_url" --header "x-paop-api-key: $api_key" || true)
  if [[ "$result" == *'recovery.checkout'* ]]; then break; fi
  sleep 1
done
[[ "$result" == *'recovery.checkout'* ]]
[[ "$result" == *'[REDACTED]'* && "$result" == *'[REDACTED_EMAIL]'* ]]
if [[ "$result" == *'recovery-secret-must-not-persist'* || "$result" == *'recovery.user@example.test'* ]]; then
  echo 'recovery test leaked a seeded secret or PII' >&2
  exit 1
fi

# Send the exact same event identity again. The ledger must prevent another
# committed effect even if Redpanda redelivers after worker recovery.
status=$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' --request POST "$gateway_url" --header 'content-type: application/json' --header "x-paop-api-key: $api_key" --data "$payload")
test "$status" = '202'
sleep 2
result=$(curl --silent --show-error "$query_url" --header "x-paop-api-key: $api_key")
count=$(grep -o '"eventKey"' <<<"$result" | wc -l | tr -d ' ')
test "$count" = '1'

logs=$("${compose[@]}" logs --no-color gateway tailer persist query)
if [[ "$logs" == *'recovery-secret-must-not-persist'* || "$logs" == *'recovery.user@example.test'* || "$logs" == *"$api_key"* ]]; then
  echo 'recovery test leaked a seeded secret, PII, or key in logs' >&2
  exit 1
fi
echo 'recovery smoke passed: staged event survived tailer restart, delayed storage, and duplicate delivery without a duplicate query effect'
