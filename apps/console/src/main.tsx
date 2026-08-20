import { useState } from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";

type Span = { eventKey: string; spanId: string; parentSpanId?: string; signalType: "trace" | "log"; name: string; ingestedAt: string; attributes: Record<string, string>; policyVersion: string; redactedPaths: string[] };
type Result = { traceId: string; spans: Span[] };
type Metrics = { windowHours: number; spanCount: number; traceCount: number; errorCount: number; logCount: number };
type Dependency = { source: string; target: string; count: number };
type DependencyResult = { windowHours: number; dependencies: Dependency[] };
type AuditEvent = { action: string; safeMetadata: Record<string, string>; createdAt: string };
type AuditResult = { events: AuditEvent[] };
type SamplingDecision = { traceId: string; reason: string; retained: boolean; spanCount: number; createdAt: string };
type SamplingResult = { decisions: SamplingDecision[] };

function App() {
  const [baseURL, setBaseURL] = useState("http://localhost:18081");
  const [apiKey, setAPIKey] = useState("");
  const [traceID, setTraceID] = useState("");
  const [result, setResult] = useState<Result | null>(null);
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [dependencies, setDependencies] = useState<DependencyResult | null>(null);
  const [audit, setAudit] = useState<AuditResult | null>(null);
  const [sampling, setSampling] = useState<SamplingResult | null>(null);
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
  const [status, setStatus] = useState("Enter a trace ID and your tenant API key. Keys remain in this browser field and are never stored.");

  function request(path: string, init: RequestInit = {}) {
    const headers = new Headers(init.headers);
    headers.set("X-PAOP-API-Key", apiKey);
    return fetch(`${baseURL.replace(/\/$/, "")}${path}`, { ...init, headers });
  }

  async function lookup(event: React.FormEvent) {
    event.preventDefault();
    setResult(null);
    setStatus("Looking up the sanitized tenant trace…");
    try {
      const response = await request(`/v1/traces/${encodeURIComponent(traceID)}`);
      if (!response.ok) throw new Error(response.status === 401 ? "The API key was not accepted." : "The trace could not be loaded safely.");
      const next = (await response.json()) as Result;
      setResult(next);
      setStatus(next.spans.length ? `${next.spans.length} sanitized span(s) found.` : "No visible spans for this tenant and trace.");
    } catch (error) { setStatus(error instanceof Error ? error.message : "The trace could not be loaded safely."); }
  }

  async function loadMetrics() {
    setStatus("Loading this tenant’s derived 24-hour metrics…");
    try {
      const response = await request("/v1/metrics");
      if (!response.ok) throw new Error(response.status === 401 ? "The API key was not accepted." : "Metrics could not be loaded safely.");
      const next = (await response.json()) as Metrics;
      setMetrics(next);
      setStatus("Derived metrics loaded. They contain counts only, never raw attributes.");
    } catch (error) { setStatus(error instanceof Error ? error.message : "Metrics could not be loaded safely."); }
  }

  async function loadDependencies() {
    setStatus("Loading this tenant’s derived service topology…");
    try {
      const response = await request("/v1/dependencies");
      if (!response.ok) throw new Error(response.status === 401 ? "The API key was not accepted." : "Service topology could not be loaded safely.");
      const next = (await response.json()) as DependencyResult;
      setDependencies(next);
      setStatus(next.dependencies.length ? "Derived service topology loaded. Edges contain no raw telemetry." : "No service-to-service edges are visible for this tenant yet.");
    } catch (error) { setStatus(error instanceof Error ? error.message : "Service topology could not be loaded safely."); }
  }

  async function loadAudit() {
    setStatus("Loading this tenant’s safe operational audit timeline…");
    try {
      const response = await request("/v1/audit");
      if (!response.ok) throw new Error(response.status === 401 ? "The API key was not accepted." : "Audit history could not be loaded safely.");
      const next = (await response.json()) as AuditResult;
      setAudit(next);
      setStatus(next.events.length ? "Safe audit timeline loaded." : "No tenant audit events are visible yet.");
    } catch (error) { setStatus(error instanceof Error ? error.message : "Audit history could not be loaded safely."); }
  }

  async function loadSampling() {
    setStatus("Loading this tenant’s bounded tail-sampling decisions…");
    try {
      const response = await request("/v1/sampling");
      if (!response.ok) throw new Error(response.status === 401 ? "The API key was not accepted." : "Sampling decisions could not be loaded safely.");
      const next = (await response.json()) as SamplingResult;
      setSampling(next);
      setStatus(next.decisions.length ? "Explicit sampling decisions loaded." : "No tail-sampling decisions are visible for this tenant yet.");
    } catch (error) { setStatus(error instanceof Error ? error.message : "Sampling decisions could not be loaded safely."); }
  }

  async function requestDeletion() {
    if (deleteConfirmation !== "DELETE_SANITIZED_TELEMETRY") {
      setStatus("Type DELETE_SANITIZED_TELEMETRY exactly before requesting deletion.");
      return;
    }
    setStatus("Requesting deletion of this tenant’s sanitized telemetry…");
    try {
      const response = await request("/v1/retention/delete", { method: "POST", headers: { "X-PAOP-Delete-Confirm": deleteConfirmation } });
      if (!response.ok) throw new Error(response.status === 401 ? "The API key was not accepted." : "Deletion could not be requested safely.");
      setResult(null); setMetrics(null); setDependencies(null); setSampling(null); setDeleteConfirmation("");
      setStatus("Deletion mutation requested. PostgreSQL tail data was removed; ClickHouse erasure completes asynchronously.");
    } catch (error) { setStatus(error instanceof Error ? error.message : "Deletion could not be requested safely."); }
  }

  return <main><header><p className="eyebrow">Privacy-Aware Observability Platform</p><h1>Trace investigation, without raw secrets.</h1><p>Every displayed attribute was sanitized before durable storage. The console never asks for, displays, or persists another tenant’s telemetry.</p></header>
    <form onSubmit={lookup}><label>Query API URL<input value={baseURL} onChange={(e) => setBaseURL(e.target.value)} inputMode="url" /></label><label>Tenant API key<input value={apiKey} onChange={(e) => setAPIKey(e.target.value)} type="password" autoComplete="off" required /></label><label>Trace ID<input value={traceID} onChange={(e) => setTraceID(e.target.value)} pattern="[A-Za-z0-9_-]{1,128}" required /></label><button>Investigate trace</button><button type="button" className="secondary" onClick={loadMetrics} disabled={!apiKey}>24-hour overview</button><button type="button" className="secondary" onClick={loadDependencies} disabled={!apiKey}>Service map</button><button type="button" className="secondary" onClick={loadAudit} disabled={!apiKey}>Audit timeline</button><button type="button" className="secondary" onClick={loadSampling} disabled={!apiKey}>Sampling decisions</button></form>
    <p className="status" aria-live="polite">{status}</p>
    {metrics && <section className="metrics" aria-label="Tenant metrics for the last 24 hours"><article><strong>{metrics.spanCount}</strong><span>sanitized spans</span></article><article><strong>{metrics.traceCount}</strong><span>visible traces</span></article><article><strong>{metrics.logCount}</strong><span>sanitized logs</span></article><article><strong>{metrics.errorCount}</strong><span>error-marked spans</span></article><article><strong>{metrics.spanCount ? `${((metrics.errorCount / metrics.spanCount) * 100).toFixed(2)}%` : "0.00%"}</strong><span>derived error rate</span></article></section>}
    {dependencies && <section className="dependencies" aria-label="Tenant service dependency map"><h2>Service dependencies · last 24 hours</h2>{dependencies.dependencies.map((edge) => <article key={`${edge.source}:${edge.target}`}><strong>{edge.source}</strong><span aria-hidden="true"> → </span><strong>{edge.target}</strong><small>{edge.count} observed edge{edge.count === 1 ? "" : "s"}</small></article>)}</section>}
    {audit && <section className="audit" aria-label="Tenant audit timeline"><h2>Safe audit timeline</h2>{audit.events.map((event, index) => <article key={`${event.action}:${event.createdAt}:${index}`}><strong>{event.action}</strong><small>{event.createdAt || "timestamp unavailable"}</small><pre>{JSON.stringify(event.safeMetadata, null, 2)}</pre></article>)}</section>}
    {sampling && <section className="audit" aria-label="Tenant tail-sampling decisions"><h2>Tail-sampling decisions</h2>{sampling.decisions.map((decision, index) => <article key={`${decision.traceId}:${decision.createdAt}:${index}`}><strong>{decision.reason}</strong><small>{decision.retained ? "retained" : "not retained"} · {decision.spanCount} record{decision.spanCount === 1 ? "" : "s"}</small><code>{decision.traceId}</code></article>)}</section>}
    {result && <section aria-label="Sanitized incident timeline"><h2>Sanitized incident timeline · {result.traceId}</h2>{result.spans.map((span) => <article key={span.eventKey}><div><strong>{span.signalType === "log" ? "Log record" : span.name}</strong><code>{span.spanId || "unlinked"}</code></div><p>{span.signalType === "log" ? "Trace-linked sanitized log" : span.parentSpanId ? `Causal parent: ${span.parentSpanId}` : "Trace root"} · Policy {span.policyVersion}; redacted fields: {span.redactedPaths.join(", ") || "none"}</p><pre>{JSON.stringify(span.attributes, null, 2)}</pre></article>)}</section>}
    <section className="deletion" aria-label="Tenant deletion workflow"><h2>Delete this tenant’s sanitized telemetry</h2><p>This deletes PostgreSQL tail metadata now and requests ClickHouse deletion asynchronously. It cannot be undone.</p><label>Type DELETE_SANITIZED_TELEMETRY<input value={deleteConfirmation} onChange={(event) => setDeleteConfirmation(event.target.value)} autoComplete="off" /></label><button type="button" className="danger" onClick={requestDeletion} disabled={!apiKey || deleteConfirmation !== "DELETE_SANITIZED_TELEMETRY"}>Request deletion</button></section>
  </main>;
}
createRoot(document.getElementById("root")!).render(<App />);
