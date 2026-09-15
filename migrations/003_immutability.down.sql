GRANT DELETE ON transactions TO ledger;
GRANT UPDATE, DELETE ON entries TO ledger;

DROP TRIGGER IF EXISTS transactions_immutable ON transactions;
DROP TRIGGER IF EXISTS transactions_no_delete ON transactions;
DROP TRIGGER IF EXISTS entries_immutable ON entries;

DROP FUNCTION IF EXISTS transactions_restrict_update();
DROP FUNCTION IF EXISTS reject_mutation();