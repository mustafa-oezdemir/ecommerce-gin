CREATE TABLE IF NOT EXISTS product_images (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  product_id BIGINT UNSIGNED NOT NULL,
  filename VARCHAR(255) NOT NULL,
  position INT UNSIGNED NOT NULL DEFAULT 0,
  UNIQUE KEY idx_product_images_filename (filename),
  KEY idx_product_images_order (product_id, position),
  CONSTRAINT fk_product_images_product FOREIGN KEY (product_id) REFERENCES products(id) ON UPDATE CASCADE ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
INSERT INTO product_images (created_at, updated_at, product_id, filename, position)
SELECT CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3), products.id, products.image_filename, 0
FROM products
WHERE products.image_filename IS NOT NULL
  AND products.image_filename <> ''
  AND NOT EXISTS (
    SELECT 1 FROM product_images WHERE product_images.product_id = products.id AND product_images.filename = products.image_filename
  );
