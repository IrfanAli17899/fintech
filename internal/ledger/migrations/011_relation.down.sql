CREATE TYPE ledger.transaction_kind AS ENUM ('topup','transfer','payment','refund','fee');

ALTER TABLE ledger.transactions
  DROP CONSTRAINT IF EXISTS transactions_relation_check,
  DROP COLUMN IF EXISTS relation,
  ALTER COLUMN kind TYPE ledger.transaction_kind USING kind::ledger.transaction_kind,
  ADD CONSTRAINT transactions_check CHECK (kind <> 'refund' OR parent_id IS NOT NULL);

DROP TYPE ledger.transaction_relation;

CREATE OR REPLACE FUNCTION ledger.transactions_restrict_update() RETURNS trigger AS $$
BEGIN
  IF NEW.id <> OLD.id
     OR NEW.kind <> OLD.kind
     OR NEW.parent_id IS DISTINCT FROM OLD.parent_id
     OR NEW.rail      IS DISTINCT FROM OLD.rail
     OR NEW.rail_ref  IS DISTINCT FROM OLD.rail_ref
     OR NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'only status, posted_at and status_reason may change on transactions'
      USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;