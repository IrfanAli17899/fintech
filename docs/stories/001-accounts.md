# Story 1 — Accounts

**Goal:** typed, per-currency buckets every entry lands in.

**Acceptance**

- POST /accounts, GET /accounts/{id}
- type ∈ asset | liability | revenue | expense; currency ISO 4217
- unique (owner_type, owner_id, currency, type) → 409 on duplicate
- system accounts cash_at_bank (asset) and fees (revenue) seeded on startup

**Decisions**

- owner_id is an opaque string, no FK — the ledger doesn't own users/merchants
- errors carry a category + code (errdefs); HTTP mapping lives only in httpx
- repos accept DBTX so the same code runs on *sql.DB or *sql.Tx (WithTx)
- database/sql over pgxpool, deliberately, to stay close to raw SQL
