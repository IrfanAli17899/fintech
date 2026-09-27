# Stories 9–10 — Reversals

**Goal:** undo all or part of a posted transaction with a new, linked transaction.

**Acceptance**

- POST /transactions/{id}/reverse with idempotency_key and optional amount (0 = the rest)
- creates a refund transaction, parent_id → original, every leg direction flipped
- original must be posted → else 400 TRANSACTION_NOT_REVERSIBLE
- cumulative reversed + amount ≤ original total → else 400 REVERSAL_EXCEEDS_ORIGINAL
- a fully reversed original stays posted; a further attempt fails REVERSAL_EXCEEDS_ORIGINAL with 0 remaining
- concurrent partials against one original cannot together exceed it

**Design**

- original row locked FOR UPDATE for the whole operation
- prior reversals summed per leg (ReversedAmountsByAccount, kind = refund only);
  each leg is scaled against its _remaining_ amount, so "reverse the rest" is
  exact and cumulative partials always land on the original per leg
- integer rounding shortfall on each side goes to that side's largest leg;
  legs that scale to zero are dropped after shortfall allocation
- reversal goes through the same post pipeline: accounts, funds check, write —
  so a merchant with insufficient balance makes the refund bounce (INSUFFICIENT_FUNDS)
- reversal-of-a-reversal is allowed; it is just a posted transaction

**Decisions**

- reversals live in transactions, not a separate table; business context
  (dispute reason, provider ref) will live in Build 2's disputes table
- proportional scaling is the ledger's default; settlement (Build 3) may pass
  explicit entries instead, e.g. to keep the platform fee on partial refunds
- three lifecycle states only: pending | posted | voided. A reversal never
  mutates the original; "fully reversed" is derived from children
  (remaining == 0). Matches Modern Treasury and TigerBeetle, where lineage is
  a link and the original transfer is immutable. Rejected: a denormalized
  `reversed` flag — cheap, but a shortcut, not the standard
- lineage is a single parent_id; kind disambiguates the relationship
  (refund → parent was reversed; posted payment with parent → capture of that hold).
  Replaces reverses_id now that a second relationship type exists
- a reversed transaction is still posted money in every sum; only voided means
  "never happened"
