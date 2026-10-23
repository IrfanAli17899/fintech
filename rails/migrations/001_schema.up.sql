-- Rails schema baseline (v0.1.0). The history that led here is in
-- docs/stories; from this baseline on, migrations are append-only.

CREATE SCHEMA IF NOT EXISTS rails;

CREATE TYPE rails.attempt_state AS ENUM ('initiated', 'settled', 'bounced', 'cancelled');
CREATE TYPE rails.outcome_kind AS ENUM ('settle', 'cancel', 'refund');

-- one movement of money through an external rail
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

-- Every decision a rail makes about an attempt, and whether the ledger applied
-- it: rails is the ledger's outbox for rail outcomes. The rail writes state;
-- the ledger acks with applied_at. Workers lease rows via claimed_until and
-- back off failed decisions via next_attempt_at.
CREATE TABLE rails.outcomes (
  ref             TEXT NOT NULL REFERENCES rails.attempts(ref),
  kind            rails.outcome_kind NOT NULL,
  key             TEXT NOT NULL DEFAULT '',          -- idempotency key for refunds; '' for settle/cancel
  amount          BIGINT NOT NULL CHECK (amount > 0),
  state           rails.attempt_state NOT NULL DEFAULT 'initiated',
  reason          TEXT,
  applied_at      TIMESTAMPTZ,
  claimed_until   TIMESTAMPTZ,
  attempt_count   INT NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ,
  last_error      TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (ref, kind, key)
);
CREATE INDEX outcomes_unapplied_idx ON rails.outcomes(updated_at)
  WHERE state <> 'initiated' AND applied_at IS NULL;
CREATE INDEX outcomes_settle_due_idx ON rails.outcomes(ref)
  WHERE kind = 'settle' AND state = 'initiated';
