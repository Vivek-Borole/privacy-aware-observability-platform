alter table telemetry.spans add column if not exists parent_span_id String after span_id;
