CREATE TYPE account_type AS ENUM ('asset','liability','revenue','expense');

CREATE TABLE accounts (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_type  TEXT NOT NULL,        -- 'customer' | 'merchant' | 'system'
  owner_id    TEXT NOT NULL,
  currency    CHAR(3) NOT NULL,
  type        account_type NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (owner_type, owner_id, currency, type)
);