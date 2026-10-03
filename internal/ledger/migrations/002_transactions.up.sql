CREATE TYPE ledger.transaction_kind AS ENUM ('topup','transfer','payment','refund','fee');
CREATE TYPE ledger.transaction_status AS ENUM ('pending','posted','voided');
CREATE TYPE ledger.entry_direction AS ENUM ('debit','credit');

CREATE TABLE ledger.transactions (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  kind            ledger.transaction_kind NOT NULL,
  status          ledger.transaction_status NOT NULL DEFAULT 'posted',
  parent_id     UUID REFERENCES ledger.transactions(id),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  posted_at       TIMESTAMPTZ,

  CHECK (kind <> 'refund' OR parent_id IS NOT NULL)
);
CREATE INDEX transactions_parent_id_idx ON ledger.transactions(parent_id) WHERE parent_id IS NOT NULL;

CREATE TABLE ledger.entries (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  transaction_id UUID NOT NULL REFERENCES ledger.transactions(id),
  account_id     UUID NOT NULL REFERENCES ledger.accounts(id),
  amount         BIGINT NOT NULL CHECK (amount > 0),
  direction      ledger.entry_direction NOT NULL,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX entries_account_id_idx ON ledger.entries(account_id);
CREATE INDEX entries_transaction_id_idx ON ledger.entries(transaction_id);