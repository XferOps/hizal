-- 027_email_verification.up.sql
-- Add email_verified column to users table
ALTER TABLE users ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT FALSE;

-- Backfill existing users so they are not locked out
UPDATE users SET email_verified = TRUE;

-- Create email_verification_tokens table
CREATE TABLE email_verification_tokens (
    token UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index on user_id for quick lookups
CREATE INDEX idx_email_verification_tokens_user_id ON email_verification_tokens(user_id);