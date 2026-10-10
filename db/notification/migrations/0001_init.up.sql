-- 0001_init.up.sql
-- Notification service schema (database-per-service: this runs against the `notification` DB).

-- Tenant-scoped message templates. `version` is the current version number; every
-- update snapshots the prior subject/body into template_versions.
CREATE TABLE templates (
  id         CHAR(36)     NOT NULL PRIMARY KEY,
  tenant_id  CHAR(36)     NOT NULL,
  name       VARCHAR(255) NOT NULL,
  channel    VARCHAR(16)  NOT NULL,                      -- email | sms
  locale     VARCHAR(16)  NOT NULL,
  subject    VARCHAR(255) NOT NULL DEFAULT '',           -- email only
  body       TEXT         NOT NULL,                      -- may contain {{var}} and {{link "url"}}
  version    INT          NOT NULL DEFAULT 1,
  status     VARCHAR(16)  NOT NULL DEFAULT 'active',     -- active | archived
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_templates_tenant (tenant_id, channel)
);

-- Immutable history for rollback.
CREATE TABLE template_versions (
  id          CHAR(36)     NOT NULL PRIMARY KEY,
  template_id CHAR(36)     NOT NULL,
  version     INT          NOT NULL,
  subject     VARCHAR(255) NOT NULL DEFAULT '',
  body        TEXT         NOT NULL,
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_tpl_version (template_id, version)
);

-- One row per notification; generalizes otp.delivery_logs. `id` is the idempotency
-- key: a duplicate send with the same id is a no-op.
CREATE TABLE notification_log (
  id               CHAR(36)     NOT NULL PRIMARY KEY,
  tenant_id        CHAR(36)     NOT NULL,
  channel          VARCHAR(16)  NOT NULL,
  recipient_masked VARCHAR(255) NOT NULL,                -- masked, never raw PII
  template_id      CHAR(36)     NULL,
  kind             VARCHAR(16)  NOT NULL,                -- transactional | marketing
  state            VARCHAR(16)  NOT NULL,                -- queued | sent | failed | suppressed
  provider         VARCHAR(32)  NULL,
  provider_msg_id  VARCHAR(128) NULL,
  latency_ms       BIGINT       NULL,
  error            TEXT         NULL,
  created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_notif_tenant (tenant_id, created_at)
);

-- Preferences / opt-out per (tenant, user, channel). No row means enabled.
CREATE TABLE notification_settings (
  tenant_id CHAR(36)     NOT NULL,
  user_ref  VARCHAR(255) NOT NULL,                       -- tenant's own user identifier
  channel   VARCHAR(16)  NOT NULL,
  enabled   BOOLEAN      NOT NULL DEFAULT TRUE,
  PRIMARY KEY (tenant_id, user_ref, channel)
);

-- Append-only engagement events.
CREATE TABLE notification_events (
  id              BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  notification_id CHAR(36)     NOT NULL,
  type            VARCHAR(16)  NOT NULL,                 -- delivered | opened | clicked
  ts              DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  meta            VARCHAR(255) NULL,
  INDEX idx_events_notif (notification_id, type)
);
