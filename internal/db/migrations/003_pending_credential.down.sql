ALTER TABLE registration_requests
    DROP COLUMN IF EXISTS pending_credential,
    DROP COLUMN IF EXISTS passkey_registered_at;
