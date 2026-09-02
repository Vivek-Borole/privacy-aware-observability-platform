# Schema and privacy-policy evolution

Every sanitized event carries an integer envelope schema and the applied
privacy-policy version. Writers emit schema v2. Readers accept v2 and the
immediately previous v1 format; legacy records without a version are treated as
v1. An unknown version is never written to ClickHouse. The consumer stores only
tenant, broker coordinates, schema number, error class, timestamps, and a
SHA-256 payload digest in `telemetry_dead_letters`—never the untrusted payload.

Tenant privacy policies are read from PostgreSQL through a 30-second cache. A
candidate policy is fully validated and each regular expression compiled before
the cache swaps atomically. A database error or invalid candidate leaves the
last valid compiled policy active. Every redaction receipt records the exact
version used, so investigations and deletion evidence remain attributable.

Migration tests must prove current/previous reads, reject future versions into
the content-free dead-letter path, and verify that an invalid policy cannot
replace the last valid version. Supporting more than one previous envelope is a
future migration project, not an implicit compatibility promise.
