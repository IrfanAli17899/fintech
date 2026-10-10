-- 003 marked every settled or bounced attempt applied, assuming the old
-- one-step worker always applied what the rail decided. Reconciliation (21c)
-- found bounces it never applied. Applying is idempotent (settle:{id} and
-- bounce:{id} keys replay), so un-ack every decided settle outcome and let the
-- worker apply each once more; only the ones never applied change anything.
UPDATE rails.outcomes
SET applied_at = NULL, claimed_until = NULL, updated_at = now()
WHERE kind = 'settle' AND state <> 'initiated';
