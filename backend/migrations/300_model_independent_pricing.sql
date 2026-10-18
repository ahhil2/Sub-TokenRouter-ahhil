ALTER TABLE pricing_config_model_pricing
    ADD COLUMN IF NOT EXISTS independent_pricing BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE pricing_config_model_pricing
    ADD CONSTRAINT pricing_config_model_independent_pricing_mode_check
    CHECK (NOT independent_pricing OR billing_mode IN ('per_request', 'image', 'video'));
