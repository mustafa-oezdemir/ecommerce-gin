ALTER TABLE order_shipments
  DROP INDEX idx_order_shipments_handover_code,
  ADD INDEX idx_order_shipments_handover_code (handover_code);
