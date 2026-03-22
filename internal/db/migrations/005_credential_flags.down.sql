ALTER TABLE webauthn_credentials
    DROP COLUMN IF EXISTS backup_eligible,
    DROP COLUMN IF EXISTS backup_state;
