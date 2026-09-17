CREATE TYPE transaction_kind AS ENUM ('topup','transfer','payment','refund','fee');
CREATE TYPE transaction_status AS ENUM ('pending','posted','voided','reversed');
CREATE TYPE entry_direction AS ENUM ('debit','credit');

CREATE TABLE transactions (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  kind            transaction_kind NOT NULL,
  status          transaction_status NOT NULL DEFAULT 'posted',
  reverses_id     UUID REFERENCES transactions(id),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  posted_at       TIMESTAMPTZ
);

CREATE TABLE entries (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  transaction_id UUID NOT NULL REFERENCES transactions(id),
  account_id     UUID NOT NULL REFERENCES accounts(id),
  amount         BIGINT NOT NULL CHECK (amount > 0),
  direction      entry_direction NOT NULL,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX entries_account_id_idx ON entries(account_id);
CREATE INDEX entries_transaction_id_idx ON entries(transaction_id);