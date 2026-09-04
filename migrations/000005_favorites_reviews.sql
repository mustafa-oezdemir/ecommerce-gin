ALTER TABLE product_lists
  ADD COLUMN system_key VARCHAR(32) NULL,
  ADD UNIQUE KEY idx_product_lists_user_system (user_id, system_key);
CREATE TABLE IF NOT EXISTS product_reviews (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  product_id BIGINT UNSIGNED NOT NULL,
  rating TINYINT UNSIGNED NOT NULL,
  title VARCHAR(150) NOT NULL,
  body TEXT NOT NULL,
  UNIQUE KEY idx_product_reviews_user_product (user_id, product_id),
  KEY idx_product_reviews_product_created (product_id, created_at),
  KEY idx_product_reviews_product_rating (product_id, rating),
  CONSTRAINT chk_product_reviews_rating CHECK (rating BETWEEN 1 AND 10),
  CONSTRAINT fk_product_reviews_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_product_reviews_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
