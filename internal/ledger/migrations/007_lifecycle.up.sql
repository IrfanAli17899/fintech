ALTER TABLE ledger.entries ADD COLUMN seq BIGSERIAL;
CREATE UNIQUE INDEX entries_seq_idx ON ledger.entries(seq);

ALTER TABLE ledger.balances
  ADD COLUMN pending_debits  BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN pending_credits BIGINT NOT NULL DEFAULT 0;