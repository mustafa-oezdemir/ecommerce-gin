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
