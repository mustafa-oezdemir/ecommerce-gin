ALTER TABLE users
  ADD COLUMN first_name VARCHAR(100) NOT NULL DEFAULT '',
  ADD COLUMN last_name VARCHAR(100) NOT NULL DEFAULT '',
  ADD COLUMN security_version BIGINT UNSIGNED NOT NULL DEFAULT 1,
  ADD COLUMN two_factor_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN two_factor_secret VARBINARY(512) NULL,
  ADD COLUMN two_factor_confirmed_at DATETIME(3) NULL;
CREATE TABLE IF NOT EXISTS recovery_codes (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  code_hash BINARY(32) NOT NULL,
  used_at DATETIME(3) NULL,
  UNIQUE KEY idx_recovery_codes_user_hash (user_id, code_hash),
  KEY idx_recovery_codes_user_unused (user_id, used_at),
  CONSTRAINT fk_recovery_codes_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS email_change_requests (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  pending_email VARCHAR(254) NOT NULL,
  code_hash BINARY(32) NOT NULL,
  expires_at DATETIME(3) NOT NULL,
  attempts INT UNSIGNED NOT NULL DEFAULT 0,
  UNIQUE KEY idx_email_change_user (user_id),
  KEY idx_email_change_pending_email (pending_email),
  KEY idx_email_change_expires (expires_at),
  CONSTRAINT fk_email_change_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
