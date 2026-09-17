# Story 6 — Balance cache

**Goal:** fast balance reads that are provably derived from entries.

**Acceptance**

- balances(account_id, debits, credits) upserted in Post's transaction
- balance endpoint reads the cache; rebuild truncates and recomputes from
  posted entries; verify reports every cache/live mismatch

**Decisions**

- cache is a pure function of entries: if it can be rebuilt, it's a cache;
  if it can't, it's a second source of truth
- rebuild uses TRUNCATE inside a tx — blocks reads/writes for its duration;
  production would build a shadow table and swap
- no as_of marker yet: entry ids are UUIDs, so "up to here" needs a
  monotonic sequence (story 11)
- verify uses FULL OUTER JOIN + COALESCE so missing rows on either side count
- /admin has no auth — deferred
- in real systems verify is a scheduled check that alerts; rebuild is a
  manual on-call action
