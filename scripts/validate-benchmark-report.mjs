#!/usr/bin/env node
// Validates the shape and release thresholds of a measured synthetic benchmark.
// It reads only synthetic result JSON and never prints a report's trace IDs.
import { readFileSync } from "node:fs";

function fail(message) {
  throw new Error(`benchmark report invalid: ${message}`);
}

function finite(value, name) {
  if (typeof value !== "number" || !Number.isFinite(value)) fail(`${name} must be a finite number`);
}

function validate(ingestion, lookup) {
  for (const [name, value] of Object.entries({
    targetPerSecond: ingestion.targetPerSecond,
    durationSeconds: ingestion.durationSeconds,
    elapsedMillis: ingestion.elapsedMillis,
    achievedPerSecond: ingestion.achievedPerSecond,
    sent: ingestion.sent,
    accepted: ingestion.accepted,
    failed: ingestion.failed,
    p50Millis: ingestion.p50Millis,
    p95Millis: ingestion.p95Millis,
    p99Millis: ingestion.p99Millis,
    lookupP50Millis: lookup.p50Millis,
    lookupP95Millis: lookup.p95Millis,
    lookupP99Millis: lookup.p99Millis,
  })) finite(value, name);
  if (ingestion.targetPerSecond < 5000) fail("target must be at least 5,000 spans/s");
  if (ingestion.durationSeconds !== 600) fail("requested duration must be 600 seconds");
  if (typeof ingestion.startedAt !== "string" || typeof ingestion.finishedAt !== "string") fail("start and finish timestamps are required");
  if (!Array.isArray(ingestion.traceSamples) || ingestion.traceSamples.length === 0) fail("at least one successful trace sample is required");
  if (ingestion.accepted !== ingestion.sent || ingestion.failed !== 0) fail("all emitted spans must be accepted without failures");
  if (ingestion.elapsedMillis < 600000) fail("actual run finished before the required 10 minutes");
  if (ingestion.achievedPerSecond < 5000) fail("actual accepted throughput is below 5,000 spans/s");
  if (lookup.lookupPass !== true || lookup.p95Millis > 3000) fail("sanitized trace lookup p95 exceeds three seconds");
}

if (process.argv[2] === "--self-test") {
  validate(
    { startedAt: "2026-01-01T00:00:00Z", finishedAt: "2026-01-01T00:10:00Z", targetPerSecond: 5000, durationSeconds: 600, elapsedMillis: 600000, achievedPerSecond: 5000, sent: 3000000, accepted: 3000000, failed: 0, p50Millis: 1, p95Millis: 2, p99Millis: 3, traceSamples: ["synthetic-id"] },
    { p50Millis: 1, p95Millis: 2, p99Millis: 3, lookupPass: true },
  );
  console.log("benchmark report validator self-test passed");
} else {
  if (process.argv.length !== 4) fail("usage: validate-benchmark-report.mjs <ingestion.json> <lookup.json>");
  const ingestion = JSON.parse(readFileSync(process.argv[2], "utf8"));
  const lookup = JSON.parse(readFileSync(process.argv[3], "utf8"));
  validate(ingestion, lookup);
  console.log("benchmark report passes required thresholds");
}
