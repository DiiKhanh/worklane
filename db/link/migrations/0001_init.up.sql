-- 0001_init.up.sql
-- Link service schema (database-per-service: this runs against the `link` DB).
CREATE TABLE links (
  id            BIGINT       NOT NULL PRIMARY KEY,          -- snowflake (pkg/idgen)
  code          VARCHAR(16)  NOT NULL,                      -- base62(id)
  tenant_id     CHAR(36)     NOT NULL,
  long_url      TEXT         NOT NULL,
  long_url_hash CHAR(64)     NOT NULL,                      -- sha256(long_url): TEXT cannot be uniquely indexed
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_links_code (code),
  UNIQUE KEY uq_links_tenant_url (tenant_id, long_url_hash), -- dedup: one code per tenant + URL
  INDEX idx_links_tenant (tenant_id, created_at)
);

-- Append-only click log. `code` is denormalized so aggregates need no join.
CREATE TABLE link_clicks (
  id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  code       VARCHAR(16)  NOT NULL,
  tenant_id  CHAR(36)     NOT NULL,
  ts         DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  referer    VARCHAR(255) NULL,
  ua         VARCHAR(255) NULL,
  ip_hash    CHAR(64)     NULL,                             -- hashed, never the raw IP
  INDEX idx_clicks_code_ts (code, ts)
);
