#!/usr/bin/env bash
set -euo pipefail

cluster_name="${PAOP_KIND_CLUSTER:-paop-v02}"
chart='deploy/helm/privacy-aware-observability-platform'

if ! kind get clusters | grep -qx "$cluster_name"; then
  kind create cluster --name "$cluster_name" --wait 90s
fi

build_and_load() {
  local component="$1"
  local target="$2"
  docker build --target "$target" -t "paop/$component:dev" .
  kind load docker-image --name "$cluster_name" "paop/$component:dev"
}

build_and_load gateway gateway
build_and_load query query
build_and_load tailer tailer
build_and_load persist persist
build_and_load migrate migrate
build_and_load clickhouse-migrate clickhouse-migrate
build_and_load bootstrap bootstrap

helm upgrade --install paop "$chart" \
  --kube-context "kind-$cluster_name" \
  --namespace paop --create-namespace \
  -f "$chart/values-kind.yaml" --wait --timeout 8m

kubectl --context "kind-$cluster_name" -n paop wait --for=condition=complete job/paop-migrate --timeout=180s
kubectl --context "kind-$cluster_name" -n paop wait --for=condition=complete job/paop-clickhouse-migrate --timeout=180s
kubectl --context "kind-$cluster_name" -n paop wait --for=condition=complete job/paop-topic-init --timeout=180s
kubectl --context "kind-$cluster_name" -n paop wait --for=condition=complete job/paop-bootstrap --timeout=180s
kubectl --context "kind-$cluster_name" -n paop rollout status deployment/paop-gateway --timeout=180s
kubectl --context "kind-$cluster_name" -n paop rollout status deployment/paop-query --timeout=180s
kubectl --context "kind-$cluster_name" -n paop rollout status deployment/paop-tailer --timeout=180s
kubectl --context "kind-$cluster_name" -n paop rollout status deployment/paop-persist --timeout=180s

api_key="$(kubectl --context "kind-$cluster_name" -n paop get secret paop-runtime -o jsonpath='{.data.api-key}' | base64 --decode)"
kubectl --context "kind-$cluster_name" -n paop port-forward service/paop-gateway 28080:8080 >/tmp/paop-kind-gateway.log 2>&1 &
port_forward_pid=$!
trap 'kill "$port_forward_pid" 2>/dev/null || true' EXIT
for _ in $(seq 1 30); do
  curl -fsS http://127.0.0.1:28080/metrics >/dev/null && break
  sleep 1
done

go run ./cmd/synthetic-emitter -endpoint http://127.0.0.1:28080/v1/traces -transport protobuf -api-key "$api_key" -rate 20 -workers 2 -batch-size 10 -duration 2s >/tmp/paop-kind-emitter.json

kubectl --context "kind-$cluster_name" -n paop rollout restart deployment/paop-gateway deployment/paop-query deployment/paop-tailer deployment/paop-persist
kubectl --context "kind-$cluster_name" -n paop rollout status deployment/paop-gateway --timeout=180s
kubectl --context "kind-$cluster_name" -n paop rollout status deployment/paop-query --timeout=180s
kubectl --context "kind-$cluster_name" -n paop rollout status deployment/paop-tailer --timeout=180s
kubectl --context "kind-$cluster_name" -n paop rollout status deployment/paop-persist --timeout=180s

gateway_ready="$(kubectl --context "kind-$cluster_name" -n paop get deployment paop-gateway -o jsonpath='{.status.readyReplicas}')"
query_ready="$(kubectl --context "kind-$cluster_name" -n paop get deployment paop-query -o jsonpath='{.status.readyReplicas}')"
if [[ "$gateway_ready" -lt 2 || "$query_ready" -lt 2 ]]; then
  echo "expected two ready gateway and query replicas" >&2
  exit 1
fi

node -e 'const r=require("/tmp/paop-kind-emitter.json"); if(r.failed!==0||r.accepted!==r.sent) process.exit(1)'
trace_id="$(node -e 'const r=require("/tmp/paop-kind-emitter.json"); process.stdout.write(r.traceSamples[0])')"
kubectl --context "kind-$cluster_name" -n paop port-forward service/paop-query 28081:8081 >/tmp/paop-kind-query.log 2>&1 &
query_forward_pid=$!
trap 'kill "$port_forward_pid" "$query_forward_pid" 2>/dev/null || true' EXIT
for _ in $(seq 1 30); do
  curl -fsS http://127.0.0.1:28081/metrics >/dev/null && break
  sleep 1
done
query_code="$(curl -sS -o /tmp/paop-kind-query.json -w '%{http_code}' -H "X-PAOP-API-Key: $api_key" "http://127.0.0.1:28081/v1/traces/$trace_id")"
if [[ "$query_code" != '200' ]]; then
  echo "expected the sanitized trace to remain queryable after rolling restart" >&2
  exit 1
fi
node -e 'const r=require("/tmp/paop-kind-query.json"); if(!Array.isArray(r.spans)||r.spans.length<1) process.exit(1)'
echo "kind smoke passed: two gateway/query replicas, native OTLP accepted, rolling restart recovered, sanitized trace remained queryable"
