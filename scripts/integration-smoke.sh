#!/usr/bin/env bash
set -euo pipefail

if docker compose version >/dev/null 2>&1; then
  compose=(docker compose)
else
  compose=(docker-compose)
fi

api_key='synthetic-integration-key-not-for-production'
postgres_url='postgres://paop:paop-local-only@127.0.0.1:5433/paop?sslmode=disable'
gateway_url='http://127.0.0.1:18080/v1/traces'
query_url='http://127.0.0.1:18081/v1/traces/smoke-trace-001'
metrics_url='http://127.0.0.1:18081/v1/metrics'
payload='{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"synthetic-gateway"}},{"key":"authorization","value":{"stringValue":"Bearer smoke-secret-must-not-persist"}}]},"scopeSpans":[{"spans":[{"traceId":"smoke-trace-001","spanId":"smoke-span-001","name":"synthetic.checkout","attributes":[{"key":"customer.email","value":{"stringValue":"smoke.user@example.test"}},{"key":"http.status_code","value":{"intValue":"200"}}]}]}]}]}'

"${compose[@]}" up -d --build postgres redpanda clickhouse migrate topic-init clickhouse-migrate persist gateway query
PAOP_POSTGRES_URL="$postgres_url" PAOP_TENANT_ID='synthetic-smoke' PAOP_API_KEY="$api_key" go run ./cmd/bootstrap >/dev/null

status=$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' --request POST "$gateway_url" --header 'content-type: application/json' --header "x-paop-api-key: $api_key" --data "$payload")
test "$status" = '202'

for _ in {1..30}; do
  result=$(curl --silent --show-error "$query_url" --header "x-paop-api-key: $api_key" || true)
  if [[ "$result" == *'synthetic.checkout'* ]]; then break; fi
  sleep 1
done
[[ "$result" == *'synthetic.checkout'* ]]
[[ "$result" == *'[REDACTED]'* ]]
[[ "$result" == *'[REDACTED_EMAIL]'* ]]
if [[ "$result" == *'smoke-secret-must-not-persist'* || "$result" == *'smoke.user@example.test'* ]]; then
  echo 'secret or PII appeared in query output' >&2
  exit 1
fi

for _ in {1..30}; do
  metrics=$(curl --silent --show-error "$metrics_url" --header "x-paop-api-key: $api_key" || true)
  if [[ "$metrics" == *'"spanCount":1'* ]]; then break; fi
  sleep 1
done
echo "derived metrics response: $metrics"
[[ "$metrics" == *'"windowHours":24'* ]]
[[ "$metrics" == *'"spanCount":1'* ]]
[[ "$metrics" == *'"traceCount":1'* ]]
[[ "$metrics" == *'"errorCount":0'* ]]

logs=$("${compose[@]}" logs --no-color gateway persist query)
if [[ "$logs" == *'smoke-secret-must-not-persist'* || "$logs" == *'smoke.user@example.test'* || "$logs" == *"$api_key"* ]]; then
  echo 'secret or PII appeared in service logs' >&2
  exit 1
fi
echo 'integration smoke passed: authenticated OTLP ingest, redaction, durable persistence, and tenant-scoped investigation metrics'
