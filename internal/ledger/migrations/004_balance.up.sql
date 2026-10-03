CREATE TABLE ledger.balances (
  account_id  UUID PRIMARY KEY REFERENCES ledger.accounts(id),
  debits      BIGINT NOT NULL DEFAULT 0,
  credits     BIGINT NOT NULL DEFAULT 0,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);