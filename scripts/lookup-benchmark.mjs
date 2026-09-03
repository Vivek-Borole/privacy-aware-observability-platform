import { readFileSync, writeFileSync } from "node:fs";

const [rawPath, outputPath] = process.argv.slice(2);
const key = process.env.PAOP_BENCHMARK_KEY;
if (!rawPath || !outputPath || !key) {
  throw new Error("usage: PAOP_BENCHMARK_KEY=<ephemeral> node scripts/lookup-benchmark.mjs <ingestion.json> <lookup.json>");
}
const raw = JSON.parse(readFileSync(rawPath, "utf8"));
const latencies = [];
for (const traceId of raw.traceSamples || []) {
  const start = performance.now();
  const response = await fetch(`http://127.0.0.1:18081/v1/traces/${traceId}`, {
    headers: { "x-paop-api-key": key },
  });
  if (!response.ok) throw new Error(`lookup failed with ${response.status}`);
  await response.arrayBuffer();
  latencies.push(performance.now() - start);
}
latencies.sort((a, b) => a - b);
const percentile = (p) => latencies.length ? latencies[Math.floor((latencies.length - 1) * p / 100)] : 0;
const report = {
  sampledTraces: latencies.length,
  p50Millis: percentile(50),
  p95Millis: percentile(95),
  p99Millis: percentile(99),
  lookupPass: percentile(95) <= 3000,
};
writeFileSync(outputPath, `${JSON.stringify(report, null, 2)}\n`, { mode: 0o600 });
console.log(JSON.stringify(report));
