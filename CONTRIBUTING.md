# Contributing

This repository is private while its release gates are incomplete. Do not use
real telemetry, credentials, or customer data in issues, test fixtures, pull
requests, screenshots, or benchmark artifacts.

## Local checks

Run these before proposing a change:

```bash
pnpm install --frozen-lockfile
pnpm console:typecheck
pnpm console:build
go vet ./...
go test -race ./...
pnpm audit --audit-level=high
```

With Docker available, also run the synthetic-only integration checks:

```bash
bash scripts/integration-smoke.sh
bash scripts/recovery-smoke.sh
bash scripts/backpressure-smoke.sh
```

## Contribution rules

- Preserve the pre-persistence redaction boundary. Do not add raw payload,
  credential, tenant-content, trace-ID, or attribute logging.
- Keep tenant authorization server-enforced. Client parameters cannot select a
  different tenant.
- Make reliability claims only when a repeatable script and its measured
  evidence exist.
- Add unit and synthetic integration coverage for a changed safety or durable
  boundary. Automated tests must never contact real services.
- Keep API changes compatible under `/v1`; use a new version for a breaking
  contract.

By contributing, you agree that your contribution may be released under the
[MIT License](LICENSE) once the project’s public-release gate is met.
