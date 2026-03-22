-- Store the WebAuthn credential from registration before admin approval.
-- The passkey is registered after email verification; admin approval creates the user.
ALTER TABLE registration_requests
    ADD COLUMN pending_credential    JSONB,
    ADD COLUMN passkey_registered_at TIMESTAMPTZ;
