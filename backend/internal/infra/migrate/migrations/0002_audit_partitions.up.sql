-- 0002_audit_partitions.sql — audit_logs declared PARTITION BY RANGE (created_at)
-- with the current-month partition and a default catch-all partition.
-- Source: docs/prd/10-data-model.md §3.9.
--
-- Partition rotation beyond the current month is owned by a later issue
-- (audit middleware / scheduler). v1 only seeds the current month so writes
-- have somewhere to land on day 1.

CREATE TABLE audit_logs (
  id             BIGINT GENERATED ALWAYS AS IDENTITY,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  method         TEXT NOT NULL,                      -- RPC 名
  actor_id       BIGINT,                             -- users.id；匿名/登录失败为 null
  actor_email    TEXT,                               -- 冗余留存，防重命名后失真
  scope_type     TEXT,                               -- workspace | project
  scope_id       BIGINT,
  resource       TEXT,                               -- 受影响资源名
  severity       TEXT NOT NULL DEFAULT 'info',
  status_code    INT,
  status_message TEXT,
  latency_ms     INT,
  request        TEXT,                               -- 已脱敏 JSON
  response       TEXT,                               -- 已脱敏 JSON
  ip             INET,
  user_agent     TEXT,
  service_data   JSONB,                              -- 如权限变更前后差异
  PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- 计算当前月起的下个月初（first day of next month, UTC）。
-- 用 date_trunc 推导，避免硬编码年月。
DO $$
DECLARE
  current_month_start DATE := date_trunc('month', NOW())::DATE;
  next_month_start    DATE := (date_trunc('month', NOW()) + INTERVAL '1 month')::DATE;
  current_partition   TEXT := format('audit_logs_%s', to_char(current_month_start, 'YYYY_MM'));
  default_partition   TEXT := 'audit_logs_default';
BEGIN
  -- 当月分区
  EXECUTE format(
    'CREATE TABLE IF NOT EXISTS %I PARTITION OF audit_logs FOR VALUES FROM (%L) TO (%L)',
    current_partition, current_month_start, next_month_start
  );
  -- 默认兜底分区（任何越界 / 历史写入落到这里，不会因分区缺失而报错）
  EXECUTE format(
    'CREATE TABLE IF NOT EXISTS %I PARTITION OF audit_logs DEFAULT',
    default_partition
  );
END
$$;

-- 审计查询性能索引（按 PRD §6）。
CREATE INDEX audit_logs_created_at_idx       ON audit_logs (created_at DESC);
CREATE INDEX audit_logs_method_idx           ON audit_logs (method);
CREATE INDEX audit_logs_actor_id_idx         ON audit_logs (actor_id);
CREATE INDEX audit_logs_resource_idx         ON audit_logs (resource);
CREATE INDEX audit_logs_scope_idx            ON audit_logs (scope_type, scope_id, created_at DESC);