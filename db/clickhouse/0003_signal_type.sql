alter table telemetry.spans add column if not exists signal_type LowCardinality(String) default 'trace' after event_id;
