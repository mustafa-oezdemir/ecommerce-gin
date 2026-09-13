ALTER TABLE payments
  ADD COLUMN payment_intent_id VARCHAR(128) NULL AFTER provider_payment_id,
  ADD COLUMN provider_transaction_id VARCHAR(128) NULL AFTER payment_intent_id,
  ADD COLUMN approval_url VARCHAR(512) NOT NULL DEFAULT '' AFTER provider_transaction_id,
  ADD UNIQUE KEY idx_payment_provider_intent (provider, payment_intent_id),
  ADD UNIQUE KEY idx_payment_provider_transaction (provider, provider_transaction_id);
