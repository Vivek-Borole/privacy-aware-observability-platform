# Privacy model

The default policy removes or replaces values before durable publication.

| Input category | Action |
| --- | --- |
| `authorization`, `proxy-authorization`, cookies, API keys, tokens | replace with `[REDACTED]` |
| emails in attribute values | replace with `[REDACTED_EMAIL]` |
| configured regular-expression matches | replace with `[REDACTED_PATTERN]` |
| non-sensitive bounded attributes | retain unchanged |

Every accepted envelope includes a policy version and a list of redacted field
paths, never removed values. Redactors are composed: a log body containing an
email and a configured pattern receives both replacements before publication.
The PostgreSQL tail buffer and outbox contain only that sanitized envelope.
Sampling decisions retain trace ID, reason, count, and timestamp only—never an
attribute or body. Tests seed bearer tokens, cookies, API keys, emails, and
configured-pattern values in traces and OTLP logs and assert their absence from
emitted envelopes, decision records, and logs.
