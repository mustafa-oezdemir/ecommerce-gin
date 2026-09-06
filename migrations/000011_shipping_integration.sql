CREATE TABLE IF NOT EXISTS order_shipments (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  deleted_at DATETIME(3) NULL,
  order_id BIGINT UNSIGNED NOT NULL,
  shipment_id VARCHAR(64) NOT NULL,
  tracking_number VARCHAR(64) NOT NULL,
  shipment_type VARCHAR(16) NOT NULL,
  status VARCHAR(50) NOT NULL,
  status_label VARCHAR(100) NOT NULL,
  remaining_stops INT NULL,
  estimated_from DATETIME(3) NULL,
  estimated_until DATETIME(3) NULL,
  last_shipping_event VARCHAR(64) NOT NULL DEFAULT '',
  UNIQUE KEY idx_order_shipments_order_id (order_id),
  UNIQUE KEY idx_order_shipments_shipment_id (shipment_id),
  UNIQUE KEY idx_order_shipments_tracking_number (tracking_number),
  KEY idx_order_shipments_deleted_at (deleted_at),
  CONSTRAINT fk_order_shipments_order FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS shipping_event_receipts (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NOT NULL,
  event_id VARCHAR(64) NOT NULL,
  UNIQUE KEY idx_shipping_event_receipts_event_id (event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
