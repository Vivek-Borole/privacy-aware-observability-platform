# Privacy model

The default policy removes or replaces values before durable publication.

| Input category | Action |
| --- | --- |
| `authorization`, `proxy-authorization`, cookies, API keys, tokens | replace with `[REDACTED]` |
| emails in attribute values | replace with `[REDACTED_EMAIL]` |
| configured regular-expression matches | replace with `[REDACTED_PATTERN]` |
| non-sensitive bounded attributes | retain unchanged |

Every accepted envelope includes a policy version and a list of redacted field paths, never removed values. Tests seed bearer tokens, cookies, API keys, and emails and assert their absence from emitted envelopes and logs.
