create table if not exists tenants (
  id text primary key,
  created_at timestamptz not null default now()
);

create table if not exists api_keys (
  id bigserial primary key,
  tenant_id text not null references tenants(id) on delete cascade,
  key_hash text not null unique,
  disabled_at timestamptz,
  created_at timestamptz not null default now()
);

create table if not exists redaction_policies (
  tenant_id text not null references tenants(id) on delete cascade,
  version text not null,
  expression text not null,
  created_at timestamptz not null default now(),
  primary key (tenant_id, version, expression)
);

create table if not exists audit_events (
  id bigserial primary key,
  tenant_id text references tenants(id) on delete set null,
  action text not null,
  safe_metadata jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);
