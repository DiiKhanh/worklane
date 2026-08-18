CREATE TABLE users (
  id            CHAR(36) PRIMARY KEY,
  tenant_id     CHAR(36) NOT NULL,
  email         VARCHAR(255) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  status        VARCHAR(16)  NOT NULL DEFAULT 'active',
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_users_tenant (tenant_id)
);
