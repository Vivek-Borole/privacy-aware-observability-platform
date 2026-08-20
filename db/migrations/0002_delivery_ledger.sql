create table if not exists delivery_ledger (
  event_key text primary key,
  tenant_id text not null references tenants(id) on delete cascade,
  status text not null check (status in ('pending', 'persisted', 'loss_evidenced')),
  attempts integer not null default 1 check (attempts > 0),
  last_error_class text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
