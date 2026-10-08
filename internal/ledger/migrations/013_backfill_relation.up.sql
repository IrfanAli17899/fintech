-- 011 added relation without a backfill. Before 011 the only child-creating
-- paths were Reverse (always kind 'refund') and Capture (inherits the parent's kind).
ALTER TABLE ledger.transactions DISABLE TRIGGER transactions_immutable;

UPDATE ledger.transactions c
SET relation = CASE WHEN c.kind = 'refund' THEN 'reverses' ELSE 'captures' END::ledger.transaction_relation
FROM ledger.transactions p
WHERE c.parent_id = p.id AND c.relation IS NULL;

ALTER TABLE ledger.transactions ENABLE TRIGGER transactions_immutable;

ALTER TABLE ledger.transactions
  ADD CONSTRAINT transactions_child_has_relation
  CHECK (parent_id IS NULL OR relation IS NOT NULL);
