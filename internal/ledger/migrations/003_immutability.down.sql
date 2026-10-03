GRANT DELETE ON ledger.transactions TO ledger;
GRANT UPDATE, DELETE ON ledger.entries TO ledger;

DROP TRIGGER IF EXISTS transactions_immutable ON ledger.transactions;
DROP TRIGGER IF EXISTS transactions_no_delete ON ledger.transactions;
DROP TRIGGER IF EXISTS entries_immutable ON ledger.entries;

DROP FUNCTION IF EXISTS transactions_restrict_update();
DROP FUNCTION IF EXISTS reject_mutation();