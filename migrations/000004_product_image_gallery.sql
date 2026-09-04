CREATE TABLE IF NOT EXISTS product_images (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  deleted_at DATETIME(3) NULL,
  product_id BIGINT UNSIGNED NOT NULL,
  filename VARCHAR(255) NOT NULL,
  position INT UNSIGNED NOT NULL,
  UNIQUE KEY idx_product_images_product_position (product_id, position),
  UNIQUE KEY idx_product_images_filename (filename),
  KEY idx_product_images_deleted_at (deleted_at),
  CONSTRAINT fk_product_images_product FOREIGN KEY (product_id) REFERENCES products(id) ON UPDATE CASCADE ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
