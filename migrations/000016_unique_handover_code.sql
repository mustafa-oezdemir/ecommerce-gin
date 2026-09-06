UPDATE order_shipments
SET handover_code = CONCAT('LEGACY-HO-', UPPER(SUBSTRING(SHA2(CONCAT(order_id, ':', shipment_id), 256), 1, 24)))
WHERE handover_code = '';

ALTER TABLE order_shipments
  ADD UNIQUE KEY idx_order_shipments_handover_code (handover_code);
