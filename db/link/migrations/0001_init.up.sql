-- Link Service owns these tables; tenant identities remain in auth-svc.
CREATE TABLE links (
  id BIGINT NOT NULL PRIMARY KEY,
  code VARCHAR(11) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  tenant_id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  long_url TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  -- Hash the original UTF-8 bytes, never a collation-normalized URL. Readers
  -- must still compare the stored URL exactly when resolving a hash match.
  long_url_hash BINARY(32) GENERATED ALWAYS AS (UNHEX(SHA2(long_url, 256))) STORED,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  disabled_at DATETIME(3) NULL,
  state VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin
    GENERATED ALWAYS AS (IF(disabled_at IS NULL, 'active', 'disabled')) STORED,
  CONSTRAINT chk_links_id CHECK (id >= 0),
  CONSTRAINT chk_links_code CHECK (
    OCTET_LENGTH(code) >= 1
    AND NOT REGEXP_LIKE(code, '[^0-9a-zA-Z]', 'c')
    AND (OCTET_LENGTH(code) = 1 OR LEFT(code, 1) <> '0')
  ),
  CONSTRAINT chk_links_url_bytes CHECK (OCTET_LENGTH(long_url) BETWEEN 1 AND 4096),
  CONSTRAINT chk_links_disabled_at CHECK (disabled_at IS NULL OR disabled_at >= created_at),
  UNIQUE KEY uq_links_code (code),
  UNIQUE KEY uq_links_tenant_url (tenant_id, long_url_hash),
  UNIQUE KEY uq_links_tenant_code (tenant_id, code),
  INDEX idx_links_tenant_created (tenant_id, created_at DESC, id DESC)
) ENGINE=InnoDB;

-- Only active -> disabled is mutable. Exact byte comparison also rejects URL
-- changes that utf8mb4_bin's trailing-space comparison could otherwise conceal.
CREATE TRIGGER links_immutable BEFORE UPDATE ON links FOR EACH ROW
BEGIN
  IF NEW.id <> OLD.id
    OR BINARY NEW.code <> BINARY OLD.code
    OR BINARY NEW.tenant_id <> BINARY OLD.tenant_id
    OR BINARY NEW.long_url <> BINARY OLD.long_url
    OR NEW.created_at <> OLD.created_at
    OR (OLD.disabled_at IS NOT NULL AND NOT (NEW.disabled_at <=> OLD.disabled_at))
  THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'link: mapping and disabled state are permanent';
  END IF;
END;

CREATE TRIGGER links_no_delete BEFORE DELETE ON links FOR EACH ROW
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'link: codes cannot be deleted or reused';

CREATE TABLE link_clicks (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  tenant_id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  code VARCHAR(11) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  ts DATETIME(3) NOT NULL,
  referrer_origin VARCHAR(512) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  device VARCHAR(7) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  CONSTRAINT chk_link_clicks_device CHECK (device IN ('desktop', 'mobile', 'tablet', 'unknown')),
  CONSTRAINT fk_link_clicks_owner FOREIGN KEY (tenant_id, code) REFERENCES links (tenant_id, code),
  INDEX idx_link_clicks_recent (tenant_id, code, ts DESC, id DESC),
  INDEX idx_link_clicks_retention (ts)
) ENGINE=InnoDB;

-- Kept independently of visit details so 30-day cleanup cannot reduce totals.
-- Day is derived from event time in UTC, never consumer processing time.
CREATE TABLE link_daily_clicks (
  tenant_id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  code VARCHAR(11) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  day DATE NOT NULL,
  clicks BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, code, day),
  CONSTRAINT chk_link_daily_clicks_count CHECK (clicks >= 0),
  CONSTRAINT fk_link_daily_clicks_owner FOREIGN KEY (tenant_id, code) REFERENCES links (tenant_id, code)
) ENGINE=InnoDB;
