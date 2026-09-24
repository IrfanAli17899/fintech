# Stories 9–10 — Reversals

**Goal:** undo all or part of a posted transaction with a new, linked transaction.

**Acceptance**

- POST /transactions/{id}/reverse with idempotency_key and optional amount (0 = the rest)
- creates a refund transaction, reverses_id → original, every leg direction flipped
- original must be posted → else 400 TRANSACTION_NOT_REVERSIBLE
- cumulative reversed + amount ≤ original total → else 400 REVERSAL_EXCEEDS_ORIGINAL
- fully reversed original gets status = reversed; a further attempt is rejected
- concurrent partials against one original cannot together exceed it

**Design**

- original row locked FOR UPDATE for the whole operation
- prior reversals summed per leg (ReversedAmountsByAccount); each leg is scaled
  against its _remaining_ amount, so "reverse the rest" is exact and cumulative
  partials always land on the original per leg
- integer rounding shortfall on each side goes to that side's largest leg
- a leg that scales to zero is dropped
- reversal goes through the same post pipeline: accounts, funds check, write —
  so a merchant with insufficient balance makes the refund bounce (INSUFFICIENT_FUNDS)
- reversal-of-a-reversal is allowed; it is just a posted transaction

**Decisions**

- reversals live in transactions, not a separate table; business context
  (dispute reason, provider ref) will live in Build 2's disputes table
- proportional scaling is the ledger's default; settlement (Build 3) may pass
  explicit entries instead, e.g. to keep the platform fee on partial refunds
- status flip on the original is a denormalized "nothing left" flag, the one
  UPDATE story 4 permits; the money movement itself is immutable
- a reversed transaction is still posted money: its entries count toward every
  balance and sum; only `voided` means "never happened". `reversed` is a
  denormalized "nothing left to reverse" flag for listing and clearer errors
