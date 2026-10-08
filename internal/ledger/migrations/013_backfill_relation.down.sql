-- data stays backfilled; reverting it to NULL would reintroduce the bug
ALTER TABLE ledger.transactions DROP CONSTRAINT IF EXISTS transactions_child_has_relation;
