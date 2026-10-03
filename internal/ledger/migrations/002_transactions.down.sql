DROP INDEX IF EXISTS transactions_parent_id_idx;
DROP TABLE IF EXISTS ledger.entries;
DROP TABLE IF EXISTS ledger.transactions;
DROP TYPE IF EXISTS ledger.entry_direction;
DROP TYPE IF EXISTS ledger.transaction_status;
DROP TYPE IF EXISTS ledger.transaction_kind;