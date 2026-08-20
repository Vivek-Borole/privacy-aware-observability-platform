# Synthetic demo and screenshot script

Use this script only after the release checks have passed and only against the
local Compose stack. It is a capture guide, not proof by itself. Do not record
or show a real API key, tenant identifier, raw telemetry payload, customer
data, browser password-manager prompt, or terminal history.

## Preparation

1. Start a clean local stack and run `bash scripts/integration-smoke.sh`.
2. Run `curl -X POST http://127.0.0.1:18090/checkout` and keep the returned
   fabricated trace ID only in the current terminal.
3. Start `pnpm console:dev`; open the local console with the synthetic Compose
   key. Keep the browser’s developer tools and address bar out of captures.
4. Verify the console shows `[REDACTED_EMAIL]`, never the synthetic source
   email. If any raw seed is visible, stop recording and treat it as a release
   failure.

## Recording order

Keep the recording under three minutes and narrate only verified behavior.

1. Show the architecture diagram from `docs/architecture.md`.
2. Show the console’s sanitized incident timeline for the fabricated checkout:
   TypeScript gateway, Go downstream service, and asynchronous worker; point
   out causal parent identifiers and redaction receipts.
3. Load the 24-hour overview and service map. State that the numbers are
   tenant-scoped derived counts, not raw events.
4. Load tail-sampling decisions and the safe audit timeline. State that a
   pressure eviction has explicit evidence rather than a silent drop.
5. Show the Grafana dashboard at `http://localhost:13000`, explaining that the
   OpenTelemetry Collector exposes only bounded internal HTTP outcome metrics.
6. Show, but do not click, the deletion control. Explain that exact typed
   confirmation deletes PostgreSQL tail metadata and only *requests* an
   asynchronous ClickHouse mutation.

## Evidence set

Capture synthetic-only screenshots after completing the script:

- `console-sanitized-timeline.png`
- `console-service-map.png`
- `console-sampling-decisions.png`
- `grafana-api-health.png`

Before attaching them to a release, search every image and its surrounding
notes for the seeded secret, seeded email, and local API key used by the smoke
test. Attach the recorded demo only after its commands and observed behavior
match the measured benchmark and failure reports.
