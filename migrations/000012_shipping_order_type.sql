ALTER TABLE order_shipments
  DROP INDEX idx_order_shipments_order_id,
  ADD UNIQUE KEY idx_order_shipments_order_type (order_id, shipment_type);
