-- migrations/007_lifecycle.down.sql
ALTER TABLE balances
  DROP COLUMN IF EXISTS pending_credits,
  DROP COLUMN IF EXISTS pending_debits;

DROP INDEX IF EXISTS entries_seq_idx;
ALTER TABLE entries DROP COLUMN IF EXISTS seq;