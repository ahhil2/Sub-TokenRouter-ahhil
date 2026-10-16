CREATE TABLE IF NOT EXISTS identity_groups (
    code varchar(32) PRIMARY KEY,
    category varchar(16) NOT NULL,
    name varchar(64) NOT NULL,
    UNIQUE (code, category),
    CHECK ((code IN ('bonus_5', 'bonus_10') AND category = 'recharge') OR
           (code = 'ahhilai' AND category = 'pricing'))
);
INSERT INTO identity_groups (code, category, name) VALUES
    ('bonus_5', 'recharge', '5%额外'),
    ('bonus_10', 'recharge', '10%额外'),
    ('ahhilai', 'pricing', 'AHHilAI') ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS user_identity_memberships (
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category varchar(16) NOT NULL,
    identity_code varchar(32) NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, category),
    FOREIGN KEY (identity_code, category) REFERENCES identity_groups(code, category)
);
CREATE INDEX IF NOT EXISTS user_identity_memberships_code_idx ON user_identity_memberships(identity_code, user_id);

CREATE TABLE IF NOT EXISTS identity_group_rates (
    identity_code varchar(32) NOT NULL REFERENCES identity_groups(code),
    group_id bigint NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    rate_multiplier numeric(20,8) NOT NULL CHECK (rate_multiplier >= 0 AND rate_multiplier < 'Infinity'::numeric),
    PRIMARY KEY (identity_code, group_id),
    CHECK (identity_code = 'ahhilai')
);

CREATE TABLE IF NOT EXISTS identity_group_changes (
    id bigserial PRIMARY KEY,
    actor_id bigint NOT NULL,
    identity_code varchar(32) NOT NULL,
    action varchar(32) NOT NULL,
    details jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE redeem_code_usages ADD COLUMN IF NOT EXISTS recharge_snapshot jsonb;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS recharge_snapshot jsonb;
