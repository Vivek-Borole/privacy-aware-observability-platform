# Local Kubernetes quick start

This profile is a free, synthetic-only operability test. It uses Colima, kind,
Helm, and locally built images; it does not create cloud infrastructure.

Prerequisites: Docker/Colima, `kind`, `kubectl`, and Helm 3. With Colima set to
six CPUs and eight GiB RAM, run:

```bash
bash scripts/kind-smoke.sh
```

The script creates an isolated `paop-v02` cluster, builds and loads the four Go
service images, installs the chart with `values-kind.yaml`, waits for migrations
and topic creation, and verifies:

- two ready gateway and query replicas;
- two coordinated tail and persistence workers;
- an authenticated official OTLP/HTTP protobuf request and tenant-scoped query;
- rolling restarts of gateway, query, tailer, and persistence deployments;
- continued visibility of the sanitized synthetic trace after those rolls.

Delete the local cluster after inspection:

```bash
kind delete cluster --name paop-v02
```

Production Helm values intentionally require external PostgreSQL, Redpanda, and
ClickHouse endpoints plus an existing Kubernetes Secret. The repository never
commits credentials. The local profile is single-node evidence and does not
claim multi-zone or production availability.
