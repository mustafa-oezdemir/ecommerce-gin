CREATE TABLE IF NOT EXISTS user_addresses (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  deleted_at DATETIME(3) NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  first_name VARCHAR(100) NOT NULL,
  last_name VARCHAR(100) NOT NULL,
  company VARCHAR(150) NOT NULL DEFAULT '',
  street VARCHAR(150) NOT NULL,
  house_number VARCHAR(30) NOT NULL,
  address_line_2 VARCHAR(150) NOT NULL DEFAULT '',
  postal_code VARCHAR(20) NOT NULL,
  city VARCHAR(100) NOT NULL,
  state VARCHAR(100) NOT NULL DEFAULT '',
  country_code CHAR(2) NOT NULL,
  phone VARCHAR(32) NOT NULL,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  KEY idx_user_addresses_user_id (user_id),
  KEY idx_user_addresses_deleted_at (deleted_at),
  CONSTRAINT fk_user_addresses_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE orders
  ADD COLUMN order_number VARCHAR(32) NULL AFTER user_id,
  ADD COLUMN idempotency_key VARCHAR(64) NULL AFTER order_number,
  ADD COLUMN payment_method VARCHAR(32) NULL AFTER status,
  ADD COLUMN currency CHAR(3) NOT NULL DEFAULT 'EUR' AFTER payment_method,
  ADD COLUMN subtotal_cents BIGINT NOT NULL DEFAULT 0 AFTER currency,
  ADD COLUMN shipping_cents BIGINT NOT NULL DEFAULT 0 AFTER subtotal_cents,
  ADD COLUMN tax_cents BIGINT NOT NULL DEFAULT 0 AFTER shipping_cents,
  ADD COLUMN discount_cents BIGINT NOT NULL DEFAULT 0 AFTER tax_cents,
  ADD UNIQUE KEY idx_orders_order_number (order_number),
  ADD UNIQUE KEY idx_order_user_key (user_id, idempotency_key);
ALTER TABLE order_items
  ADD COLUMN product_sku VARCHAR(100) NOT NULL DEFAULT '' AFTER product_name;
CREATE TABLE IF NOT EXISTS order_addresses (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NOT NULL,
  order_id BIGINT UNSIGNED NOT NULL,
  type VARCHAR(16) NOT NULL,
  first_name VARCHAR(100) NOT NULL,
  last_name VARCHAR(100) NOT NULL,
  company VARCHAR(150) NOT NULL DEFAULT '',
  street VARCHAR(150) NOT NULL,
  house_number VARCHAR(30) NOT NULL,
  address_line_2 VARCHAR(150) NOT NULL DEFAULT '',
  postal_code VARCHAR(20) NOT NULL,
  city VARCHAR(100) NOT NULL,
  state VARCHAR(100) NOT NULL DEFAULT '',
  country_code CHAR(2) NOT NULL,
  phone VARCHAR(32) NOT NULL,
  UNIQUE KEY idx_order_address_type (order_id, type),
  CONSTRAINT fk_order_addresses_order FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS payments (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  deleted_at DATETIME(3) NULL,
  order_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  provider VARCHAR(32) NOT NULL,
  method VARCHAR(32) NOT NULL,
  provider_payment_id VARCHAR(128) NOT NULL,
  status VARCHAR(32) NOT NULL,
  amount_cents BIGINT NOT NULL,
  currency CHAR(3) NOT NULL,
  idempotency_key VARCHAR(64) NOT NULL,
  brand VARCHAR(32) NOT NULL DEFAULT '',
  last4 CHAR(4) NOT NULL DEFAULT '',
  UNIQUE KEY idx_payments_order_id (order_id),
  UNIQUE KEY idx_payment_user_key (user_id, idempotency_key),
  UNIQUE KEY idx_payment_provider_id (provider, provider_payment_id),
  KEY idx_payments_user_id (user_id),
  KEY idx_payments_status (status),
  KEY idx_payments_deleted_at (deleted_at),
  CONSTRAINT fk_payments_order FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE RESTRICT,
  CONSTRAINT fk_payments_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT,
  CONSTRAINT chk_payments_amount_positive CHECK (amount_cents > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS checkout_attempts (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  idempotency_key VARCHAR(64) NOT NULL,
  request_hash CHAR(64) NOT NULL,
  status VARCHAR(20) NOT NULL,
  order_id BIGINT UNSIGNED NULL,
  UNIQUE KEY idx_checkout_user_key (user_id, idempotency_key),
  UNIQUE KEY idx_checkout_order_id (order_id),
  CONSTRAINT fk_checkout_attempts_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_checkout_attempts_order FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS webhook_events (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NOT NULL,
  provider VARCHAR(32) NOT NULL,
  event_id VARCHAR(128) NOT NULL,
  UNIQUE KEY idx_webhook_provider_event (provider, event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE products
  ADD CONSTRAINT chk_products_stock_nonnegative CHECK (stock >= 0);
