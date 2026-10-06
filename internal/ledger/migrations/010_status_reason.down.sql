ALTER TABLE ledger.transactions DROP COLUMN IF EXISTS status_reason;

CREATE OR REPLACE FUNCTION ledger.transactions_restrict_update() RETURNS trigger AS $$
BEGIN
  IF NEW.id <> OLD.id
     OR NEW.kind <> OLD.kind
     OR NEW.parent_id IS DISTINCT FROM OLD.parent_id
     OR NEW.rail      IS DISTINCT FROM OLD.rail
     OR NEW.rail_ref  IS DISTINCT FROM OLD.rail_ref
     OR NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'only status and posted_at may change on transactions'
      USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;