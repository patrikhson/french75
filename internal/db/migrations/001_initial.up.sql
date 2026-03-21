CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================
-- DRINKS
-- ============================================================
CREATE TABLE drinks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    description TEXT,
    recipe      TEXT,
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    added_by    UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO drinks (name, description) VALUES
    ('French 75', 'Gin, lemon juice, simple syrup, topped with Champagne.');

-- ============================================================
-- DRINK REQUESTS
-- ============================================================
CREATE TYPE drink_request_status AS ENUM ('pending', 'approved', 'rejected');

CREATE TABLE drink_requests (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    requested_by UUID NOT NULL,
    name         TEXT NOT NULL,
    description  TEXT,
    reason       TEXT,
    status       drink_request_status NOT NULL DEFAULT 'pending',
    reviewed_by  UUID,
    review_note  TEXT,
    reviewed_at  TIMESTAMPTZ,
    drink_id     UUID REFERENCES drinks(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_drink_requests_status ON drink_requests(status);

-- ============================================================
-- USERS  (no passwords — auth via IDPs)
-- ============================================================
CREATE TYPE user_role AS ENUM ('passive', 'active', 'admin');

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT NOT NULL UNIQUE,
    display_name  TEXT,
    avatar_url    TEXT,
    role          user_role NOT NULL DEFAULT 'passive',
    bio           TEXT,
    checkin_count INTEGER NOT NULL DEFAULT 0,
    is_banned     BOOLEAN NOT NULL DEFAULT FALSE,
    invited_by    UUID REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_role     ON users(role);

ALTER TABLE drink_requests
    ADD CONSTRAINT fk_drink_requests_user  FOREIGN KEY (requested_by) REFERENCES users(id),
    ADD CONSTRAINT fk_drink_requests_admin FOREIGN KEY (reviewed_by)  REFERENCES users(id);

-- ============================================================
-- USER IDENTITIES  (one user can link multiple IDPs)
-- ============================================================
CREATE TABLE user_identities (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider         TEXT NOT NULL,         -- 'google', 'freja', 'webauthn'
    provider_subject TEXT NOT NULL,         -- 'sub' from OIDC, or credential ID for WebAuthn
    provider_email   TEXT,
    linked_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_subject)
);

CREATE INDEX idx_user_identities_user_id ON user_identities(user_id);

-- ============================================================
-- WEBAUTHN CREDENTIALS  (Yubikey / passkeys)
-- ============================================================
CREATE TABLE webauthn_credentials (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id BYTEA NOT NULL UNIQUE,
    public_key    BYTEA NOT NULL,
    aaguid        BYTEA,
    sign_count    BIGINT NOT NULL DEFAULT 0,
    name          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at  TIMESTAMPTZ
);

CREATE INDEX idx_webauthn_creds_user_id ON webauthn_credentials(user_id);

-- ============================================================
-- INVITES
-- ============================================================
CREATE TABLE invites (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token      TEXT NOT NULL UNIQUE,
    email      TEXT NOT NULL,
    invited_by UUID NOT NULL REFERENCES users(id),
    used_by    UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
    used_at    TIMESTAMPTZ
);

CREATE INDEX idx_invites_token ON invites(token);

-- ============================================================
-- SESSIONS
-- ============================================================
CREATE TABLE sessions (
    id           TEXT PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ip_address   INET,
    user_agent   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days'
);

CREATE INDEX idx_sessions_user_id    ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

-- ============================================================
-- CHECK-INS
-- ============================================================
CREATE TYPE checkin_status AS ENUM ('pending', 'public', 'spam');

CREATE TABLE check_ins (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    drink_id            UUID NOT NULL REFERENCES drinks(id),
    score               SMALLINT NOT NULL CHECK (score >= 0 AND score <= 100),
    review              TEXT NOT NULL,
    drink_date          DATE NOT NULL,
    status              checkin_status NOT NULL DEFAULT 'pending',
    location_name       TEXT NOT NULL,
    location_lat        DOUBLE PRECISION NOT NULL,
    location_lng        DOUBLE PRECISION NOT NULL,
    location_osm_id     BIGINT,
    location_osm_type   TEXT,
    submission_lat      DOUBLE PRECISION,
    submission_lng      DOUBLE PRECISION,
    submission_accuracy DOUBLE PRECISION,
    exif_timestamp      TIMESTAMPTZ,
    exif_check_passed   BOOLEAN,
    gps_check_passed    BOOLEAN,
    gps_distance_m      INTEGER,
    edit_deadline       TIMESTAMPTZ NOT NULL,
    like_count          INTEGER NOT NULL DEFAULT 0,
    helpful_count       INTEGER NOT NULL DEFAULT 0,
    flag_count          INTEGER NOT NULL DEFAULT 0,
    submitted_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_checkins_user_id      ON check_ins(user_id);
CREATE INDEX idx_checkins_drink_id     ON check_ins(drink_id);
CREATE INDEX idx_checkins_status       ON check_ins(status);
CREATE INDEX idx_checkins_submitted_at ON check_ins(submitted_at DESC);
CREATE INDEX idx_checkins_lat          ON check_ins(location_lat);
CREATE INDEX idx_checkins_lng          ON check_ins(location_lng);

-- ============================================================
-- PHOTOS
-- ============================================================
CREATE TABLE photos (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    checkin_id     UUID REFERENCES check_ins(id) ON DELETE CASCADE,
    user_id        UUID NOT NULL REFERENCES users(id),
    storage_path   TEXT NOT NULL,
    thumbnail_path TEXT,
    mime_type      TEXT NOT NULL DEFAULT 'image/jpeg',
    size_bytes     INTEGER,
    width_px       INTEGER,
    height_px      INTEGER,
    exif_timestamp TIMESTAMPTZ,
    exif_gps_lat   DOUBLE PRECISION,
    exif_gps_lng   DOUBLE PRECISION,
    sort_order     SMALLINT NOT NULL DEFAULT 0,
    uploaded_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_photos_checkin_id ON photos(checkin_id);
CREATE INDEX idx_photos_sort_order ON photos(checkin_id, sort_order);

-- ============================================================
-- FOLLOWS
-- ============================================================
CREATE TABLE follows (
    follower_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    following_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (follower_id, following_id),
    CHECK (follower_id != following_id)
);

CREATE INDEX idx_follows_follower_id  ON follows(follower_id);
CREATE INDEX idx_follows_following_id ON follows(following_id);

-- ============================================================
-- REACTIONS
-- ============================================================
CREATE TYPE reaction_type AS ENUM ('like', 'helpful');

CREATE TABLE reactions (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    checkin_id UUID NOT NULL REFERENCES check_ins(id) ON DELETE CASCADE,
    type       reaction_type NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, checkin_id, type)
);

CREATE INDEX idx_reactions_checkin_id ON reactions(checkin_id);

-- ============================================================
-- SPAM FLAGS
-- ============================================================
CREATE TABLE spam_flags (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    checkin_id  UUID NOT NULL REFERENCES check_ins(id) ON DELETE CASCADE,
    flagged_by  UUID NOT NULL REFERENCES users(id),
    reason      TEXT,
    reviewed_by UUID REFERENCES users(id),
    reviewed_at TIMESTAMPTZ,
    resolution  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (checkin_id, flagged_by)
);

CREATE INDEX idx_spamflags_checkin_id ON spam_flags(checkin_id);
CREATE INDEX idx_spamflags_unreviewed ON spam_flags(reviewed_at) WHERE reviewed_at IS NULL;

-- ============================================================
-- TRIGGERS
-- ============================================================
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE OR REPLACE FUNCTION sync_checkin_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' AND NEW.status = 'public' THEN
        UPDATE users SET
            checkin_count = checkin_count + 1,
            role = CASE WHEN checkin_count + 1 >= 10 AND role = 'passive'
                        THEN 'active'::user_role ELSE role END
        WHERE id = NEW.user_id;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.status != 'public' AND NEW.status = 'public' THEN
            UPDATE users SET
                checkin_count = checkin_count + 1,
                role = CASE WHEN checkin_count + 1 >= 10 AND role = 'passive'
                            THEN 'active'::user_role ELSE role END
            WHERE id = NEW.user_id;
        ELSIF OLD.status = 'public' AND NEW.status != 'public' THEN
            UPDATE users SET checkin_count = GREATEST(checkin_count - 1, 0)
            WHERE id = NEW.user_id;
        END IF;
    ELSIF TG_OP = 'DELETE' AND OLD.status = 'public' THEN
        UPDATE users SET checkin_count = GREATEST(checkin_count - 1, 0)
        WHERE id = OLD.user_id;
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER checkin_count_sync
    AFTER INSERT OR UPDATE OF status OR DELETE ON check_ins
    FOR EACH ROW EXECUTE FUNCTION sync_checkin_count();
