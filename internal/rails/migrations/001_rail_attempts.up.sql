CREATE SCHEMA IF NOT EXISTS rails;

CREATE TYPE rails.attempt_state AS ENUM ('initiated', 'settled', 'bounced', 'reversed');

CREATE TABLE rails.attempts (
  ref            TEXT PRIMARY KEY,
  rail           TEXT NOT NULL,
  transaction_id UUID NOT NULL,                 -- correlation only; no FK into ledger
  amount         BIGINT NOT NULL,
  currency       CHAR(3) NOT NULL,
  state          rails.attempt_state NOT NULL DEFAULT 'initiated',
  settle_after   TIMESTAMPTZ NOT NULL,
  reason         TEXT,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX attempts_due_idx ON rails.attempts(settle_after) WHERE state = 'initiated';
CREATE INDEX attempts_transaction_idx ON rails.attempts(transaction_id);