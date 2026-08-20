create table if not exists tail_traces (
  tenant_id text not null references tenants(id) on delete cascade,
  trace_id text not null,
  first_seen timestamptz not null default now(),
  last_seen timestamptz not null default now(),
  generation bigint not null default 1,
  span_count integer not null default 0 check (span_count >= 0),
  lease_owner text,
  lease_expires_at timestamptz,
  decision text check (decision in ('retained_error', 'retained_slow', 'retained_healthy_sample', 'dropped_healthy_sample', 'evicted_pressure', 'evicted_span_limit')),
  decided_at timestamptz,
  primary key (tenant_id, trace_id)
);

create index if not exists tail_traces_ready_idx on tail_traces (last_seen)
  where decided_at is null;

create table if not exists tail_buffers (
  event_key text primary key,
  tenant_id text not null,
  trace_id text not null,
  envelope jsonb not null,
  received_at timestamptz not null default now(),
  foreign key (tenant_id, trace_id) references tail_traces(tenant_id, trace_id) on delete cascade
);

create index if not exists tail_buffers_trace_idx on tail_buffers (tenant_id, trace_id, received_at);

create table if not exists tail_decisions (
  id bigserial primary key,
  tenant_id text not null references tenants(id) on delete cascade,
  trace_id text not null,
  reason text not null check (reason in ('retained_error', 'retained_slow', 'retained_healthy_sample', 'dropped_healthy_sample', 'evicted_pressure', 'evicted_span_limit')),
  retained boolean not null,
  span_count integer not null check (span_count >= 0),
  created_at timestamptz not null default now(),
  unique (tenant_id, trace_id)
);

create table if not exists tail_outbox (
  event_key text primary key,
  tenant_id text not null references tenants(id) on delete cascade,
  envelope jsonb not null,
  attempts integer not null default 0 check (attempts >= 0),
  lease_owner text,
  lease_expires_at timestamptz,
  published_at timestamptz,
  created_at timestamptz not null default now()
);

create index if not exists tail_outbox_ready_idx on tail_outbox (created_at)
  where published_at is null;
