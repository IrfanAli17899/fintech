# Story 4 — Append-only ledger

**Goal:** the database refuses to rewrite history.

**Acceptance**

- UPDATE/DELETE on entries rejected by trigger
- DELETE on transactions rejected; UPDATE allowed only for status, posted_at
- UPDATE/DELETE privileges revoked from the app role

**Decisions**

- triggers guard against bugs, revoke guards against roles — both kept
- corrections are always a new reversing transaction (story 9)
