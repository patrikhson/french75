-- ============================================================
-- REGISTRATION REQUESTS
-- ============================================================
CREATE TYPE registration_status AS ENUM ('pending', 'approved', 'rejected', 'completed');

CREATE TABLE registration_requests (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token                   TEXT NOT NULL UNIQUE,
    name                    TEXT NOT NULL,
    email                   TEXT NOT NULL UNIQUE,
    email_verified          BOOLEAN NOT NULL DEFAULT FALSE,
    email_verified_at       TIMESTAMPTZ,
    status                  registration_status NOT NULL DEFAULT 'pending',
    passkey_token           TEXT UNIQUE,
    passkey_token_expires_at TIMESTAMPTZ,
    webauthn_session        JSONB,
    user_id                 UUID REFERENCES users(id),
    expires_at              TIMESTAMPTZ NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_reg_requests_email  ON registration_requests(email);
CREATE INDEX idx_reg_requests_token  ON registration_requests(token);
CREATE INDEX idx_reg_requests_status ON registration_requests(status);

-- ============================================================
-- WEBAUTHN LOGIN SESSIONS  (short-lived, one per user)
-- ============================================================
CREATE TABLE webauthn_login_sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    session_data JSONB NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_wa_login_sessions_user_id ON webauthn_login_sessions(user_id);
