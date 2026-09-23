# Story 11 — Lifecycle states

**Goal:** a transaction can hold funds (pending) before it moves them (posted).

**Acceptance**

- POST /transactions accepts status pending or posted (default posted)
- pending: posted*at NULL, sums land in pending*\* columns, balance unchanged
- entries.seq gives a strict, monotonic order
- verify/rebuild cover all four sums

**Decisions**

- pending sums as columns on balances, not a holds table: the overdraft check
  needs posted and pending in one locked read; the hold's own detail
  (expiry, rail ref) lives on the pending transaction row itself
- status is a domain input restricted to pending|posted; voided/reversed are
  transitions owned by the service, never accepted from clients
- one liveBalancesCTE constant is the definition of truth for both rebuild and verify
- same model as TigerBeetle's debits_pending/debits_posted/credits_pending/credits_posted
