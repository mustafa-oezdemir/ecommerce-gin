CREATE TABLE IF NOT EXISTS saved_payment_cards (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    created_at DATETIME(3) NULL,
    updated_at DATETIME(3) NULL,
    deleted_at DATETIME(3) NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    provider_token VARCHAR(128) NOT NULL,
    brand VARCHAR(32) NOT NULL,
    last4 CHAR(4) NOT NULL,
    exp_month TINYINT UNSIGNED NOT NULL,
    exp_year SMALLINT UNSIGNED NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT fk_saved_payment_cards_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE KEY idx_saved_card_user_token (user_id, provider_token),
    UNIQUE KEY idx_saved_card_identity (user_id, brand, last4, exp_month, exp_year),
    INDEX idx_saved_payment_cards_deleted_at (deleted_at),
    INDEX idx_saved_payment_cards_user (user_id)
);
