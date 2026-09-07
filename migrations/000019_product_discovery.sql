CREATE TABLE IF NOT EXISTS brands (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  deleted_at DATETIME(3) NULL,
  name VARCHAR(120) NOT NULL,
  UNIQUE KEY idx_brands_name (name),
  KEY idx_brands_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE products
  ADD COLUMN brand_id BIGINT UNSIGNED NULL,
  ADD KEY idx_products_brand_id (brand_id),
  ADD KEY idx_products_price_cents (price_cents),
  ADD CONSTRAINT fk_products_brand FOREIGN KEY (brand_id) REFERENCES brands(id) ON UPDATE CASCADE ON DELETE SET NULL;
CREATE TABLE IF NOT EXISTS product_variants (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  deleted_at DATETIME(3) NULL,
  product_id BIGINT UNSIGNED NOT NULL,
  sku VARCHAR(100) NOT NULL,
  color VARCHAR(80) NOT NULL DEFAULT '',
  clothing_size VARCHAR(30) NOT NULL DEFAULT '',
  shoe_size VARCHAR(30) NOT NULL DEFAULT '',
  stock INT NOT NULL DEFAULT 0,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  UNIQUE KEY idx_product_variants_sku (sku),
  KEY idx_product_variants_product_id (product_id),
  KEY idx_product_variants_active (active),
  KEY idx_product_variants_color (color),
  KEY idx_product_variants_clothing_size (clothing_size),
  KEY idx_product_variants_shoe_size (shoe_size),
  CONSTRAINT fk_product_variants_product FOREIGN KEY (product_id) REFERENCES products(id) ON UPDATE CASCADE ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
INSERT INTO product_variants (created_at, updated_at, product_id, sku, stock, active)
SELECT CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3), id, CONCAT('PRODUCT-', id), stock, active
FROM products
WHERE deleted_at IS NULL;
CREATE TABLE IF NOT EXISTS product_variant_attributes (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  deleted_at DATETIME(3) NULL,
  product_variant_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(80) NOT NULL,
  value VARCHAR(120) NOT NULL,
  KEY idx_product_variant_attributes_deleted_at (deleted_at),
  KEY idx_variant_attribute (product_variant_id, name, value),
  CONSTRAINT fk_variant_attributes_variant FOREIGN KEY (product_variant_id) REFERENCES product_variants(id) ON UPDATE CASCADE ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
