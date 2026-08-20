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
log_payload='{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"synthetic-gateway"}}]},"scopeLogs":[{"logRecords":[{"timeUnixNano":"123456","traceId":"smoke-trace-001","spanId":"smoke-span-001","severityText":"ERROR","body":{"stringValue":"smoke.user@example.test customer-77"},"attributes":[{"key":"cookie","value":{"stringValue":"session=smoke-secret-must-not-persist"}}]}]}]}]}'

"${compose[@]}" up -d --build postgres redpanda clickhouse migrate topic-init clickhouse-migrate persist gateway query prometheus grafana synthetic-worker synthetic-downstream synthetic-api
PAOP_POSTGRES_URL="$postgres_url" PAOP_TENANT_ID='synthetic-smoke' PAOP_API_KEY="$api_key" go run ./cmd/bootstrap >/dev/null

status=$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' --request POST "$gateway_url" --header 'content-type: application/json' --header "x-paop-api-key: $api_key" --data "$payload")
test "$status" = '202'
status=$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' --request POST 'http://127.0.0.1:18080/v1/logs' --header 'content-type: application/json' --header "x-paop-api-key: $api_key" --data "$log_payload")
test "$status" = '202'

for _ in {1..30}; do
  result=$(curl --silent --show-error "$query_url" --header "x-paop-api-key: $api_key" || true)
  if [[ "$result" == *'synthetic.checkout'* ]]; then break; fi
  sleep 1
done
[[ "$result" == *'synthetic.checkout'* ]]
[[ "$result" == *'"signalType":"log"'* ]]
[[ "$result" == *'[REDACTED]'* ]]
[[ "$result" == *'[REDACTED_EMAIL]'* ]]
if [[ "$result" == *'smoke-secret-must-not-persist'* || "$result" == *'smoke.user@example.test'* ]]; then
  echo 'secret or PII appeared in query output' >&2
  exit 1
fi

demo=$(curl --silent --show-error --request POST 'http://127.0.0.1:18090/checkout')
demo_trace=$(sed -n 's/.*"traceId":"\([a-f0-9]\{32\}\)".*/\1/p' <<<"$demo")
[[ -n "$demo_trace" ]]
for _ in {1..30}; do
  demo_result=$(curl --silent --show-error "http://127.0.0.1:18081/v1/traces/$demo_trace" --header "x-paop-api-key: synthetic-compose-key-not-for-production" || true)
  if [[ "$demo_result" == *'synthetic-typescript-gateway'* && "$demo_result" == *'synthetic-go-downstream'* && "$demo_result" == *'synthetic-async-worker'* ]]; then break; fi
  sleep 1
done
[[ "$demo_result" == *'synthetic-typescript-gateway'* ]]
[[ "$demo_result" == *'synthetic-go-downstream'* ]]
[[ "$demo_result" == *'synthetic-async-worker'* ]]
[[ "$demo_result" == *'"parentSpanId"'* ]]
[[ "$demo_result" == *'[REDACTED_EMAIL]'* ]]
if [[ "$demo_result" == *'synthetic.user@example.test'* ]]; then
  echo 'synthetic demo PII appeared in query output' >&2
  exit 1
fi
dependencies=$(curl --silent --show-error 'http://127.0.0.1:18081/v1/dependencies' --header 'x-paop-api-key: synthetic-compose-key-not-for-production')
[[ "$dependencies" == *'synthetic-typescript-gateway'* ]]
[[ "$dependencies" == *'synthetic-go-downstream'* ]]
[[ "$dependencies" == *'synthetic-async-worker'* ]]

for _ in {1..30}; do
  metrics=$(curl --silent --show-error "$metrics_url" --header "x-paop-api-key: $api_key" || true)
  if [[ "$metrics" == *'"spanCount":1'* && "$metrics" == *'"logCount":1'* ]]; then break; fi
  sleep 1
done
echo "derived metrics response: $metrics"
[[ "$metrics" == *'"windowHours":24'* ]]
[[ "$metrics" == *'"spanCount":1'* ]]
[[ "$metrics" == *'"traceCount":1'* ]]
[[ "$metrics" == *'"logCount":1'* ]]
[[ "$metrics" == *'"errorCount":0'* ]]

logs=$("${compose[@]}" logs --no-color gateway persist query)
if [[ "$logs" == *'smoke-secret-must-not-persist'* || "$logs" == *'smoke.user@example.test'* || "$logs" == *'synthetic.user@example.test'* || "$logs" == *"$api_key"* ]]; then
  echo 'secret or PII appeared in service logs' >&2
  exit 1
fi
gateway_metrics=$(curl --silent --show-error http://127.0.0.1:18080/metrics)
query_metrics=$(curl --silent --show-error http://127.0.0.1:18081/metrics)
[[ "$gateway_metrics" == *'paop_http_requests_total'* ]]
[[ "$query_metrics" == *'paop_http_requests_total'* ]]
if [[ "$gateway_metrics$query_metrics" == *'smoke-secret-must-not-persist'* || "$gateway_metrics$query_metrics" == *'smoke.user@example.test'* || "$gateway_metrics$query_metrics" == *"$api_key"* ]]; then
  echo 'unsafe metric label content detected' >&2
  exit 1
fi
echo 'integration smoke passed: authenticated OTLP ingest, redaction, durable persistence, and tenant-scoped investigation metrics'
