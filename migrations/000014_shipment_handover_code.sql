ALTER TABLE order_shipments
  ADD COLUMN handover_code VARCHAR(64) NOT NULL DEFAULT '' AFTER shipment_id;
