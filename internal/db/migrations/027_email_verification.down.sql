-- 027_email_verification.down.sql
-- Drop email_verification_tokens table
DROP TABLE IF EXISTS email_verification_tokens;

-- Drop email_verified column from users table
ALTER TABLE users DROP COLUMN IF EXISTS email_verified;