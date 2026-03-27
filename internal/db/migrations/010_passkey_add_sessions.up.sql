-- Temporary session data for adding a new passkey to an existing account.
CREATE TABLE webauthn_add_sessions (
    user_id      UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    session_data JSONB NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
