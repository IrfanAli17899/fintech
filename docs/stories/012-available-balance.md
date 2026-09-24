# Story 12 — Available balance

**Goal:** holds reserve funds; promised funds are not spendable.

**Acceptance**

- GET /accounts/{id}/balance → posted, pending_in, pending_out, available
- available = posted − pending_out; pending_in never counts
- overdraft check uses available and includes the transaction's own pending debits
- a hold of 500 on a 1000 wallet blocks a 600 transfer and allows a 400 one
- 20 concurrent 100-unit holds against 1000 → exactly 10 succeed

**Design**

- Balance derives all four figures from AccountSums by account type; "in" is
  the direction that grows the account (credit for liability/revenue, debit
  for asset/expense), "out" is the direction that shrinks it
- checkFunds locks every touched balance row in sorted order, then checks only
  liability accounts being debited, against available

**Decisions**

- asymmetric pending: a pending debit is committed from the payer's side and
  must reduce available immediately, or the hold protects nothing; a pending
  credit is a counterparty's promise (card auth, inbound wire) that can still
  fail, so it is not funds until posted
- four separate figures instead of a signed net pending: clearer for clients
  and answers "why isn't my deposit available" without a transaction list
- attempted: lock only debited accounts to relieve the merchant hot row.
  Rejected by the opposite-direction deadlock test: Apply updates every touched
  balance row, and an UPDATE takes the same row lock, so locking a subset up
  front just acquires locks out of order. The synchronous upsert _is_ the lock;
  the hot-row remedies are async/batched credit posting or sharding, not
  locking less. All touched rows are locked, sorted, before any write.
- a reversed transaction still counts as posted money in every sum; only
  voided means "never happened" (surfaced by verify during this story's tests)
