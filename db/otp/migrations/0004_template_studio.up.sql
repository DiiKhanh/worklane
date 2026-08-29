-- 0004_template_studio.up.sql
-- Promote the unused single-row `templates` table into a versioned two-table model.
-- The old table has no writer in the codebase and is empty in all environments.
DROP TABLE IF EXISTS templates;

CREATE TABLE templates (
  id                CHAR(36)     NOT NULL PRIMARY KEY,
  name              VARCHAR(120) NOT NULL,
  channel           VARCHAR(16)  NOT NULL,
  locale            VARCHAR(16)  NOT NULL,
  status            VARCHAR(16)  NOT NULL DEFAULT 'active',
  active_version_id CHAR(36)     NULL,
  created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_templates_channel_locale (channel, locale)
);

CREATE TABLE template_versions (
  id          CHAR(36)     NOT NULL PRIMARY KEY,
  template_id CHAR(36)     NOT NULL,
  version_no  INT          NOT NULL,
  subject     VARCHAR(255) NOT NULL DEFAULT '',
  body        TEXT         NOT NULL,
  status      VARCHAR(16)  NOT NULL DEFAULT 'draft',
  note        VARCHAR(255) NOT NULL DEFAULT '',
  created_by  VARCHAR(120) NOT NULL DEFAULT '',
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_versions_template_no (template_id, version_no),
  INDEX idx_versions_template (template_id)
);

-- Seed the current env-config templates as the first published versions, so the DB is
-- the source of truth from day one (env-config remains only as the dispatcher fallback).
INSERT INTO templates (id, name, channel, locale, status, active_version_id) VALUES
  ('11111111-1111-1111-1111-111111111111', 'OTP email', 'email', 'en', 'active', '11111111-0000-0000-0000-000000000001'),
  ('22222222-2222-2222-2222-222222222222', 'OTP SMS',   'sms',   'en', 'active', '22222222-0000-0000-0000-000000000001');

INSERT INTO template_versions (id, template_id, version_no, subject, body, status, note, created_by) VALUES
  ('11111111-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 1,
   'Your verification code', 'Your verification code is {{code}}. It expires in {{expiry}}.', 'published', 'seed', 'system'),
  ('22222222-0000-0000-0000-000000000001', '22222222-2222-2222-2222-222222222222', 1,
   '', 'Your verification code is {{code}}. It expires in {{expiry}}.', 'published', 'seed', 'system');
