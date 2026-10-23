-- Ledger schema baseline (v0.1.0). The history that led here is in
-- docs/stories; from this baseline on, migrations are append-only.

CREATE SCHEMA IF NOT EXISTS ledger;

CREATE TYPE ledger.account_type AS ENUM ('asset', 'liability', 'revenue', 'expense');
CREATE TYPE ledger.transaction_status AS ENUM ('pending', 'posted', 'voided');
CREATE TYPE ledger.entry_direction AS ENUM ('debit', 'credit');
-- lineage is the ledger's; kind is the product's own word (free text)
CREATE TYPE ledger.transaction_relation AS ENUM ('captures', 'reverses');

CREATE TABLE ledger.accounts (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_type  TEXT NOT NULL,        -- 'customer' | 'merchant' | 'system'
  owner_id    TEXT NOT NULL,
  currency    CHAR(3) NOT NULL,
  type        ledger.account_type NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (owner_type, owner_id, currency, type)
);

CREATE TABLE ledger.transactions (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  kind          TEXT NOT NULL,
  status        ledger.transaction_status NOT NULL DEFAULT 'posted',
  parent_id     UUID REFERENCES ledger.transactions(id),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  posted_at     TIMESTAMPTZ,
  expires_at    TIMESTAMPTZ,               -- pending only: when an internal hold lapses
  rail          TEXT NOT NULL DEFAULT '',  -- '' or 'internal' = never left the ledger
  rail_ref      TEXT NOT NULL DEFAULT '',  -- the rail's reference for this transfer
  status_reason TEXT,                      -- why it is in its status: expired, bounced: ...
  relation      ledger.transaction_relation,
  CONSTRAINT transactions_relation_check CHECK (relation IS NULL OR parent_id IS NOT NULL),
  CONSTRAINT transactions_child_has_relation CHECK (parent_id IS NULL OR relation IS NOT NULL)
);
CREATE INDEX transactions_parent_id_idx ON ledger.transactions(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX transactions_parent_relation_idx
  ON ledger.transactions(parent_id, relation) WHERE parent_id IS NOT NULL;
CREATE INDEX transactions_pending_expiry_idx ON ledger.transactions(expires_at) WHERE status = 'pending';

CREATE TABLE ledger.entries (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  transaction_id UUID NOT NULL REFERENCES ledger.transactions(id),
  account_id     UUID NOT NULL REFERENCES ledger.accounts(id),
  amount         BIGINT NOT NULL CHECK (amount > 0),   -- minor units
  direction      ledger.entry_direction NOT NULL,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  seq            BIGSERIAL                             -- stable cursor for statements
);
CREATE INDEX entries_account_id_idx ON ledger.entries(account_id);
CREATE INDEX entries_transaction_id_idx ON ledger.entries(transaction_id);
CREATE UNIQUE INDEX entries_seq_idx ON ledger.entries(seq);

-- a rebuildable cache of per-account sums; verified against entries
CREATE TABLE ledger.balances (
  account_id      UUID PRIMARY KEY REFERENCES ledger.accounts(id),
  debits          BIGINT NOT NULL DEFAULT 0,
  credits         BIGINT NOT NULL DEFAULT 0,
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  pending_debits  BIGINT NOT NULL DEFAULT 0,
  pending_credits BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE ledger.idempotency_keys (
  key           TEXT PRIMARY KEY,
  request_hash  TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id   UUID,                   -- null until the operation completes
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- raw sums per transaction; domain.Summarize derives Remaining and Settlement
CREATE VIEW ledger.transaction_summaries AS
SELECT
  t.id,
  COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'debit'), 0) AS total,
  COALESCE((
    SELECT SUM(ce.amount)
    FROM ledger.transactions c
    JOIN ledger.entries ce ON ce.transaction_id = c.id
    WHERE c.parent_id = t.id AND c.relation = 'captures' AND c.status = 'posted'
      AND ce.direction = 'debit'
  ), 0) AS captured,
  COALESCE((
    SELECT SUM(re.amount)
    FROM ledger.transactions r
    JOIN ledger.entries re ON re.transaction_id = r.id
    WHERE r.parent_id = t.id AND r.relation = 'reverses' AND r.status = 'posted'
      AND re.direction = 'credit'
  ), 0) AS reversed
FROM ledger.transactions t
JOIN ledger.entries e ON e.transaction_id = t.id
GROUP BY t.id;

-- History is append-only: entries never change, transactions are never
-- deleted, and only status, posted_at and status_reason may change on one.
CREATE FUNCTION ledger.reject_mutation() RETURNS trigger AS $$
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

CREATE FUNCTION ledger.transactions_restrict_update() RETURNS trigger AS $$
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

CREATE TRIGGER transactions_immutable
  BEFORE UPDATE ON ledger.transactions
  FOR EACH ROW EXECUTE FUNCTION ledger.transactions_restrict_update();

REVOKE UPDATE, DELETE ON ledger.entries FROM fintech;
REVOKE DELETE ON ledger.transactions FROM fintech;
