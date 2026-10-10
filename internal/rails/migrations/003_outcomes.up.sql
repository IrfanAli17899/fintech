CREATE TYPE rails.outcome_kind AS ENUM ('settle', 'cancel', 'refund');

-- Every decision a rail makes about an attempt, and whether the ledger applied it.
-- The rail writes state; the ledger writes applied_at. Workers lease rows via
-- claimed_until and back off failed decisions via next_attempt_at.
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

-- history: the single-step worker both decided and applied every settled or
-- bounced attempt. A reversed attempt settled first; its chargeback was posted
-- by the ledger directly and has no refund outcome.
INSERT INTO rails.outcomes (ref, kind, amount, state, reason, applied_at, created_at, updated_at)
SELECT ref, 'settle', amount,
       CASE WHEN state = 'reversed' THEN 'settled'::rails.attempt_state ELSE state END,
       reason, updated_at, created_at, updated_at
FROM rails.attempts
WHERE state IN ('settled', 'bounced', 'reversed');

-- in flight: still waiting for the rail to decide
INSERT INTO rails.outcomes (ref, kind, amount, state, created_at, updated_at)
SELECT ref, 'settle', amount, 'initiated', created_at, updated_at
FROM rails.attempts
WHERE state = 'initiated';
