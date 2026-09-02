create table if not exists telemetry_dead_letters (
  id bigserial primary key,
  tenant_id text references tenants(id) on delete cascade,
  event_key text,
  schema_version integer,
  reason text not null check (reason in ('unknown_schema_version', 'malformed_envelope')),
  payload_sha256 text not null,
  broker_partition integer not null,
  broker_offset bigint not null,
  created_at timestamptz not null default now(),
  unique (broker_partition, broker_offset)
);

create index if not exists telemetry_dead_letters_tenant_created_idx
  on telemetry_dead_letters (tenant_id, created_at desc);
