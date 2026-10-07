CREATE TYPE ledger.transaction_relation AS ENUM ('captures', 'reverses');

ALTER TABLE ledger.transactions
  DROP CONSTRAINT IF EXISTS transactions_check,
  ALTER COLUMN kind TYPE TEXT,
  ADD COLUMN relation ledger.transaction_relation,
  ADD CONSTRAINT transactions_relation_check CHECK (relation IS NULL OR parent_id IS NOT NULL);

DROP TYPE ledger.transaction_kind;

CREATE INDEX transactions_parent_relation_idx
  ON ledger.transactions(parent_id, relation) WHERE parent_id IS NOT NULL;

CREATE OR REPLACE FUNCTION ledger.transactions_restrict_update() RETURNS trigger AS $$
BEGIN
  IF NEW.id <> OLD.id
     OR NEW.kind <> OLD.kind
     OR NEW.parent_id IS DISTINCT FROM OLD.parent_id
     OR NEW.relation  IS DISTINCT FROM OLD.relation
     OR NEW.rail      IS DISTINCT FROM OLD.rail
     OR NEW.rail_ref  IS DISTINCT FROM OLD.rail_ref
     OR NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'only status, posted_at and status_reason may change on transactions'
      USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;