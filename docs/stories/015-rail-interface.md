# Story 15 — The Rail port

**Goal:** the ledger moves money through the outside world without knowing which provider it's talking to.

**Acceptance**

- ledger/api.Rail: Name, Properties, Initiate, Settle, Reverse; RailRegistry resolves by name
- POST /transactions accepts optional rail; omitted = internal (instant, reversible)
- the rail's Initiate decides lifecycle: immediate → posted, deferred → pending with
  expiry = settlement delay + grace
- rail and rail_ref stored on the transaction, frozen by the immutability trigger
- unknown rail → 400 UNKNOWN_RAIL; rail failure → RAIL_FAILED, nothing written
- reversing a transaction whose rail is not reversible → 400 RAIL_NOT_REVERSIBLE

**Design**

- the contract lives in the consumer's api package (ledger/api); rails is a
  separate module that implements it and is injected in main — the ledger never
  imports rails
- Initiate runs inside the posting tx, after account resolution and before the
  funds check, so a rejected post never leaves an orphaned provider call
- transaction ids are generated in Go (UUIDv7) so the rail gets a stable
  reference before the row exists
- expiry precedence: client-supplied → rail delay + grace → DefaultHoldTTL
- tests swap in a fake registry; the fake is a map with a Get method

**Decisions**

- consumer-owned interface for now; if a second module needs the identical
  contract it moves to rails/api (provider-owned). Either way the contract
  package imports no implementation
- api types use plain strings (Kind string, Settlement enum) so the contract
  is proto-ready and independent of ledger internals
- the "provider succeeded, our commit failed" gap is left for the reconciler
  (story 21); Initiate must be idempotent on TransactionID so retries are safe
- migrations are append-only from here: 009 redefines the trigger function
  rather than editing 003

**Addendum:** the Rail port moved from `ledger/api` to `ledger/ports` in the layout refactor; see docs/ARCHITECTURE.md.
