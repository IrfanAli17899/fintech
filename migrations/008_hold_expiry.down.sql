DROP INDEX IF EXISTS transactions_pending_expiry_idx;
ALTER TABLE transactions DROP COLUMN IF EXISTS expires_at;