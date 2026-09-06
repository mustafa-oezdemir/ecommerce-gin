ALTER TABLE orders
  ADD COLUMN customer_received_at DATETIME(3) NULL AFTER status;

CREATE TABLE IF NOT EXISTS return_requests (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  deleted_at DATETIME(3) NULL,
  order_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  reason VARCHAR(32) NOT NULL,
  note VARCHAR(1000) NOT NULL DEFAULT '',
  original_shipment_id VARCHAR(64) NOT NULL,
  return_shipment_id VARCHAR(64) NOT NULL,
  return_tracking_number VARCHAR(64) NOT NULL,
  status VARCHAR(50) NOT NULL,
  UNIQUE KEY idx_return_requests_order_id (order_id),
  UNIQUE KEY idx_return_requests_original_shipment_id (original_shipment_id),
  UNIQUE KEY idx_return_requests_return_shipment_id (return_shipment_id),
  UNIQUE KEY idx_return_requests_return_tracking_number (return_tracking_number),
  KEY idx_return_requests_user_id (user_id),
  KEY idx_return_requests_deleted_at (deleted_at),
  CONSTRAINT fk_return_requests_order FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE,
  CONSTRAINT fk_return_requests_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS return_items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  deleted_at DATETIME(3) NULL,
  return_request_id BIGINT UNSIGNED NOT NULL,
  order_item_id BIGINT UNSIGNED NOT NULL,
  product_id BIGINT UNSIGNED NOT NULL,
  quantity INT NOT NULL,
  UNIQUE KEY idx_return_items_request_order_item (return_request_id, order_item_id),
  KEY idx_return_items_deleted_at (deleted_at),
  CONSTRAINT fk_return_items_request FOREIGN KEY (return_request_id) REFERENCES return_requests(id) ON DELETE CASCADE,
  CONSTRAINT fk_return_items_order_item FOREIGN KEY (order_item_id) REFERENCES order_items(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
