CREATE TABLE IF NOT EXISTS product_lists (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(100) NOT NULL,
  UNIQUE KEY idx_product_lists_user_name (user_id, name),
  KEY idx_product_lists_user_id (user_id),
  CONSTRAINT fk_product_lists_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS product_list_items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  product_list_id BIGINT UNSIGNED NOT NULL,
  product_id BIGINT UNSIGNED NOT NULL,
  UNIQUE KEY idx_product_list_item (product_list_id, product_id),
  KEY idx_product_list_items_product_id (product_id),
  CONSTRAINT fk_product_list_items_list FOREIGN KEY (product_list_id) REFERENCES product_lists(id) ON DELETE CASCADE,
  CONSTRAINT fk_product_list_items_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
