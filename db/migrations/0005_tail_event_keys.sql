create table if not exists tail_event_keys (
  event_key text primary key,
  tenant_id text not null references tenants(id) on delete cascade,
  trace_id text not null,
  created_at timestamptz not null default now()
);

create index if not exists tail_event_keys_tenant_created_idx on tail_event_keys (tenant_id, created_at);
