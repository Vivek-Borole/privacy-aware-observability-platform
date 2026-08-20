create table if not exists tenant_retention (
  tenant_id text primary key references tenants(id) on delete cascade,
  retention_days integer not null default 30 check (retention_days between 1 and 3650),
  updated_at timestamptz not null default now()
);
