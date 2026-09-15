# Story 5 — Balance from entries

**Goal:** balance is derived, never stored.

**Acceptance**

- GET /accounts/{id}/balance → credits−debits or debits−credits by account type
- only posted transactions count
- GET /accounts?owner_type=&owner_id= lists an owner's accounts

**Decisions**

- normal-balance convention: asset/expense debit-positive, liability/revenue credit-positive
- read endpoint is a snapshot (two reads, no tx); the exact, locked version
  arrives in story 8 for overdraft checks
- unknown account type panics — it's corrupted state, not a user error
