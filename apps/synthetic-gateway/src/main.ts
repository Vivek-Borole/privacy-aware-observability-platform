// Fabricated TypeScript API hop for the local-only distributed trace demo.
import { createServer, IncomingMessage, ServerResponse } from "node:http";
import { randomBytes } from "node:crypto";

const ingestURL = required("PAOP_INGEST_URL").replace(/\/$/, "");
const apiKey = required("PAOP_SYNTHETIC_API_KEY");
const downstreamURL = required("PAOP_SYNTHETIC_DOWNSTREAM_URL").replace(/\/$/, "");

function required(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`missing ${name}`);
  return value;
}
function id(bytes: number): string { return randomBytes(bytes).toString("hex"); }
async function emit(traceId: string, spanId: string): Promise<void> {
  const payload = { resourceSpans: [{ resource: { attributes: [{ key: "service.name", value: { stringValue: "synthetic-typescript-gateway" } }] }, scopeSpans: [{ spans: [{ traceId, spanId, name: "POST /checkout", attributes: [
    { key: "http.method", value: { stringValue: "POST" } },
    { key: "http.status_code", value: { intValue: "202" } },
    { key: "peer.service", value: { stringValue: "synthetic-go-downstream" } },
    { key: "customer.email", value: { stringValue: "synthetic.user@example.test" } }
  ] }] }] }] };
  const response = await fetch(`${ingestURL}/v1/traces`, { method: "POST", headers: { "content-type": "application/json", "x-paop-api-key": apiKey }, body: JSON.stringify(payload), signal: AbortSignal.timeout(3000) });
  if (response.status !== 202) throw new Error("telemetry not accepted");
}
async function checkout(request: IncomingMessage, response: ServerResponse): Promise<void> {
  if (request.method !== "POST" || request.url !== "/checkout") { response.statusCode = 404; response.end(); return; }
  const traceId = id(16);
  const spanId = id(8);
  try {
    await emit(traceId, spanId);
    const downstream = await fetch(`${downstreamURL}/checkout`, { method: "POST", headers: { "x-synthetic-trace-id": traceId, "x-synthetic-parent-span-id": spanId }, signal: AbortSignal.timeout(3000) });
    if (downstream.status !== 202) throw new Error("downstream unavailable");
    response.statusCode = 202; response.setHeader("content-type", "application/json"); response.end(JSON.stringify({ traceId, status: "accepted" }));
  } catch {
    response.statusCode = 503; response.end("synthetic demo unavailable");
  }
}
createServer((request, response) => { void checkout(request, response); }).listen(8090, () => console.log("synthetic TypeScript gateway listening on :8090"));
