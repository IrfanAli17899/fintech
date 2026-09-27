# Story 2 — Post a transaction

**Goal:** record N entries atomically; reject anything that doesn't balance.

**Acceptance**

- POST /transactions, GET /transactions/{id}
- ≥2 entries, sum(debits) == sum(credits), all accounts same currency
- no account may net to zero within the transaction
- transaction + entries inserted in one DB tx
- idempotency_key UNIQUE → 409 TRANSACTION_EXISTS
  (superseded in story 7: key moved to its own idempotency_keys table with replay semantics)

**Decisions**

- entries inserted with unnest() — fixed param count, cached plan
- reverses_id is a column, not a link table; revisit if a second relation appears
  (superseded before story 13: renamed to parent_id once capture became the second relation)
- GET loads transaction and entries in two indexed queries, not a join
- net-zero check is hygiene, not security; authorization is a later layer
