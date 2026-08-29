-- 0004_template_studio.down.sql
DROP TABLE IF EXISTS template_versions;
DROP TABLE IF EXISTS templates;

-- Restore the original (unused) shape so the down path is a true inverse.
CREATE TABLE templates (
  id CHAR(36) PRIMARY KEY,
  channel VARCHAR(16) NOT NULL,
  locale VARCHAR(16) NOT NULL,
  subject VARCHAR(255) NOT NULL,
  body TEXT NOT NULL
);
