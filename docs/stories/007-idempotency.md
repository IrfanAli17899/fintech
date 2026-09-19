# Story 7 — Idempotent posting

**Goal:** a retried request never creates a second transaction.

**Acceptance**

- same key + same payload → 200 with the original transaction
- same key + different payload → 400 IDEMPOTENCY_KEY_REUSED
- N concurrent requests with one key → exactly one transaction, all get it back

**Design**

- idempotency_keys(key PK, request_hash, resource_type, resource_id)
  in its own table so capture/void/refund can reuse it
- key claimed with INSERT ... ON CONFLICT DO NOTHING RETURNING as the
  first statement of the posting tx; empty RETURNING = conflict
- request_hash = sha256 of kind + sorted entries; whitespace/order-insensitive
- resource_id set as the last statement of the same tx, so a committed key
  always points at a committed transaction

**Decisions**

- keys in Postgres, not Redis: same tx as the entries, no second store to reconcile
- ON CONFLICT DO NOTHING instead of catching 23505: a Postgres error aborts
  the whole tx, which would kill the loser's replay lookup
- the second inserter blocks on the winner's uncommitted row, so by the time
  it sees the conflict the winner has committed — no in-flight window
- reused-key error is 400 (Validation), not 422; code carries the meaning
- keys are global for now; per-owner scoping when auth exists
- no TTL; keys live as long as transactions
