CREATE OR REPLACE FUNCTION ledger.reject_mutation() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'ledger tables are append-only: % on % not allowed', TG_OP, TG_TABLE_NAME
    USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER entries_immutable
  BEFORE UPDATE OR DELETE ON ledger.entries
  FOR EACH ROW EXECUTE FUNCTION ledger.reject_mutation();

CREATE TRIGGER transactions_no_delete
  BEFORE DELETE ON ledger.transactions
  FOR EACH ROW EXECUTE FUNCTION ledger.reject_mutation();

CREATE OR REPLACE FUNCTION ledger.transactions_restrict_update() RETURNS trigger AS $$
BEGIN
  IF NEW.id <> OLD.id
     OR NEW.kind <> OLD.kind
     OR NEW.parent_id IS DISTINCT FROM OLD.parent_id
     OR NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'only status and posted_at may change on transactions'
      USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER transactions_immutable
  BEFORE UPDATE ON ledger.transactions
  FOR EACH ROW EXECUTE FUNCTION ledger.transactions_restrict_update();

REVOKE UPDATE, DELETE ON ledger.entries FROM fintech;
REVOKE DELETE ON ledger.transactions FROM fintech;