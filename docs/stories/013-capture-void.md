# Story 13 — Capture and void

**Goal:** resolve a hold into money (capture) or release it (void), immutably.

**Acceptance**

- POST /transactions/{id}/capture with idempotency_key and optional amount
  (0 = everything still held); POST /transactions/{id}/void
- capture posts a child (same kind, parent_id → hold) scaled against what
  remains per leg, and releases only that share of the reservation
- multiple partial captures allowed until the hold is exhausted, then voided
- capture over remaining → 400 CAPTURE_EXCEEDS_HOLD
- void releases the rest and voids; captures already posted stay
- either on a non-pending transaction → 400 TRANSACTION_NOT_PENDING
- both idempotent; concurrent captures of a full hold: one wins
- verify is clean after every combination

**Design**

- hold locked FOR UPDATE; consumed-so-far read from posted children of the
  same kind (ChildAmountsByAccount), exactly as reversals do
- child entries = hold.Scaled(amount, capturedBy); reservation released via
  the child's own sums treated as pending (pendingSums), before post() so
  the funds check sees the right available
- hold never becomes posted: its entries are never money; children carry it.
  voided means "reservation released", whether by capture or cancel

**Decisions**

- multi-capture over single-capture, for symmetry with partial reversals
  (Adyen-style; Stripe's default is single)
- the live-balances definition of truth had to change: a pending hold's
  reservation is its entries minus its posted children. verify caught the
  discrepancy on the first partial-capture test — this is what the cache
  check is for
- capture/void are commands, not Post with parent_id: the server derives
  entries and lineage under a lock; clients never assert them
