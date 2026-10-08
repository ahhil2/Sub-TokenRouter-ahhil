CREATE TABLE IF NOT EXISTS site_message_conversations (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    last_message_id BIGINT NOT NULL DEFAULT 0,
    user_read_message_id BIGINT NOT NULL DEFAULT 0,
    admin_read_message_id BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS site_message_dispatches (
    id BIGSERIAL PRIMARY KEY,
    sender_id BIGINT NOT NULL,
    client_id UUID NOT NULL,
    payload_hash VARCHAR(64) NOT NULL,
    recipient_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (sender_id, client_id)
);

CREATE TABLE IF NOT EXISTS site_messages (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES site_message_conversations(user_id) ON DELETE CASCADE,
    sender_id BIGINT NOT NULL,
    is_admin BOOLEAN NOT NULL,
    dispatch_id BIGINT NOT NULL REFERENCES site_message_dispatches(id),
    body TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 4000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, dispatch_id)
);

CREATE INDEX IF NOT EXISTS idx_site_messages_thread ON site_messages (user_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_site_messages_unread ON site_messages (user_id, is_admin, id);
CREATE INDEX IF NOT EXISTS idx_site_message_conversations_recent ON site_message_conversations (last_message_id DESC);
