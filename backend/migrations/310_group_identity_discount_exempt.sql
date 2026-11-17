-- 310_group_identity_discount_exempt.sql adds each group's original-price policy.
-- Existing and new groups retain identity discounts by default. Administrators
-- can disable the two membership discounts while personal and AHHilAI overrides
-- retain their existing precedence in effective-rate queries.

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS identity_discount_exempt BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.identity_discount_exempt IS
    'Skip bonus_5 and bonus_10 balance discounts; personal and AHHilAI rates retain precedence';
