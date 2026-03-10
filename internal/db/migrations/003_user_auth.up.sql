-- Add password_hash to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash VARCHAR(255);

-- Add indexes for login lookup
CREATE INDEX IF NOT EXISTS users_email_idx ON users (email);
