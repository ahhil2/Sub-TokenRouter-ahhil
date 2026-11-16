-- 309_subscription_refund_hold.sql records the refund operation that temporarily
-- suspends a source-order subscription grant while its external refund settles.
-- Existing grants retain a NULL marker; old application instances ignore this
-- column, so every instance must be upgraded before refund operations resume.
ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS refund_hold_operation_id text;
