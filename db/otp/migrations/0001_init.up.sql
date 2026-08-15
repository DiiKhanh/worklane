CREATE TABLE tenants (
  id CHAR(36) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE api_keys (
  id CHAR(36) PRIMARY KEY,
  tenant_id CHAR(36) NOT NULL,
  hashed_key VARCHAR(128) NOT NULL UNIQUE,
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_api_keys_tenant (tenant_id)
);

CREATE TABLE otp_requests (
  id CHAR(36) PRIMARY KEY,
  tenant_id CHAR(36) NOT NULL,
  recipient_masked VARCHAR(255) NOT NULL,
  channel VARCHAR(16) NOT NULL,
  state VARCHAR(16) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_requests_tenant (tenant_id, created_at)
);

CREATE TABLE delivery_logs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  request_id CHAR(36) NOT NULL,
  tenant_id CHAR(36) NOT NULL,
  provider VARCHAR(32) NOT NULL,
  status VARCHAR(16) NOT NULL,
  latency_ms BIGINT NOT NULL,
  error TEXT,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_logs_tenant (tenant_id, created_at)
);

CREATE TABLE templates (
  id CHAR(36) PRIMARY KEY,
  channel VARCHAR(16) NOT NULL,
  locale VARCHAR(16) NOT NULL,
  subject VARCHAR(255) NOT NULL,
  body TEXT NOT NULL
);
