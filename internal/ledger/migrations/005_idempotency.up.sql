CREATE TABLE ledger.idempotency_keys (
  key           TEXT PRIMARY KEY,
  request_hash  TEXT NOT NULL,
  resource_type TEXT NOT NULL,          -- 'transaction', later 'capture', 'void'
  resource_id   UUID,                   -- null until the operation completes
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
