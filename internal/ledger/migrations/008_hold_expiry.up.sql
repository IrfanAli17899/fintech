ALTER TABLE ledger.transactions ADD COLUMN expires_at TIMESTAMPTZ;
CREATE INDEX transactions_pending_expiry_idx ON ledger.transactions(expires_at) WHERE status = 'pending';