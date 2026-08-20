create database if not exists telemetry;

create table if not exists telemetry.spans (
  tenant_id String,
  event_key String,
  event_id String,
  signal_type LowCardinality(String) default 'trace',
  trace_id String,
  span_id String,
  parent_span_id String,
  name String,
  attributes_json String,
  policy_version String,
  redacted_paths Array(String),
  ingested_at DateTime64(3, 'UTC')
) engine = ReplacingMergeTree(ingested_at)
order by (tenant_id, event_key);
