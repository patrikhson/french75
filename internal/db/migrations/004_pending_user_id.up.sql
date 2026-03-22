-- Stable user ID generated at request time.
-- Used as the WebAuthn user handle during passkey registration so it matches
-- the user account ID created at approval time.
ALTER TABLE registration_requests
    ADD COLUMN pending_user_id UUID NOT NULL DEFAULT gen_random_uuid();
