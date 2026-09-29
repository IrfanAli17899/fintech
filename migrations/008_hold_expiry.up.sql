ALTER TABLE transactions ADD COLUMN expires_at TIMESTAMPTZ;
CREATE INDEX transactions_pending_expiry_idx ON transactions(expires_at) WHERE status = 'pending';