-- 306_identity_discount_names.sql renames the bonus_5 and bonus_10 identity groups to
-- match their new meaning: members pay 95% and 90% of a group's default balance
-- multiplier instead of receiving recharge gifts. Codes, categories and memberships are
-- unchanged, so existing assignments keep working.
UPDATE identity_groups SET name = '95折' WHERE code = 'bonus_5';
UPDATE identity_groups SET name = '9折' WHERE code = 'bonus_10';
