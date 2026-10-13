# Story 20: The ledger's contract

**Goal:** other modules can use the ledger without seeing how it works, and
the code says so as plainly as the architecture doc does.

Part 1 (d13dbab) added summaries and statements. Part 2 is the contract and
the application shape behind it, the last architecture work before v0.1.0.

**Acceptance: part 1, summaries and history**

- every transaction read carries a summary: total, captured, reversed,
  remaining, and a settlement label (see 21b)
- `GET /accounts/{id}/transactions`: the account's entries newest first,
  cursor on `entries.seq`; each entry carries created_at and posted_at

**Acceptance: part 2, the contract**

- `ledger/api`: `Ledger` for products, `LedgerAdmin` for operators; plain
  value types; the pending semantics of rail-backed writes documented on the
  interface
- `ledger/codes`: every error code a caller can receive, a leaf package.
  Two codes no path ever returned were removed
- the adapter is the only package that maps domain types to api types;
  transport depends on api only; `Module.API()` hands out `api.Ledger` and
  there is no accessor for the admin surface
- every write requires an idempotency key. Before this, keyless writes all
  claimed the empty key, so the second one failed as a key reuse
- Void returns the summary a read would show, not zeros
- `internal/archtest` fails the build when a dependency rule is broken

**Design**

- one `Service` struct with one file per use case, prefixed by what it acts
  on. The struct is a dependency bag; splitting it into Account, Ledger and
  Admin services gated nothing the api interfaces do not already gate
- no nested packages under application: in Go a folder is a package, so
  nesting would force exporting the shared posting pipeline
- `application/port.UnitOfWork` replaces a raw transaction runner, so use
  cases never see `database/sql`. `Stores` lives in persistence, not port:
  the Postgres unit of work builds it and port imports persistence, so
  putting it in port would be an import cycle
- business rules move out of SQL: `domain.Summarize` derives Remaining and
  the settlement label from raw sums the view returns, with a unit test that
  needs no database
