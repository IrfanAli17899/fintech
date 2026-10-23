# Architecture

One Go module, one Postgres, one schema per bounded-context module.
Modules talk only through exported contract packages; implementation is
hidden under `internal/` and enforced by the compiler. The rules the compiler
cannot see are enforced by `internal/archtest`.

## Status

Architecture settled at v0.1.0. Everything below is true in the code.

- planned features: the wallet module (first consumer of `api.Ledger`)
- known limits, deliberately out of scope for now: no authentication on any
  endpoint; no metrics or tracing; rails are simulators; the outbox (see
  Decisions) is not built, so a rail can hold an attempt the ledger never
  committed, which reconciliation reports as `orphan_attempt`; no
  chargebacks yet: the card rail supports merchant refunds, and
  rail-initiated reversals arrive with disputes in Build 2

## Layout

Business modules sit at the root, one folder each; `cmd/` holds binaries,
`platform/` shared infrastructure, and the root `internal/` what is private to
this repo.

```
cmd/server/main.go               composition root: migrate, wire, routes, jobs, shutdown

<module>/                        ledger/, rails/; later wallet/, identity/ ...
  api/                           inbound contract: what the module offers; consumers import
  ports/                         outbound contracts: what the module needs; implementers import
  codes/                         error codes a caller can receive; leaf, zero deps
  migrations/  migrate.go        schema <module>.*, embedded, own tracking table
  module.go                      New(deps) *Module; API(); RegisterRoutes; Start/Stop; Seed
  internal/
    domain/                      entities, invariants, derived views, error constructors
    application/                 one Service; one file per use case, prefixed by what it
                                 acts on (account_, transaction_, admin_), background
                                 jobs in job_, shared helpers in shared_
      port/                      UnitOfWork, the only abstracted dependency
    persistence/                 Postgres repos (concrete), Stores, the Postgres UnitOfWork
    adapter/                     api over application; the only domain <-> api mapping
    transport/http/              handlers + JSON DTOs; depend on api only

internal/archtest/               tests that enforce the dependency rules below
platform/                        infrastructure with no module knowledge: database, errdefs,
                                 httpx, scheduler
```

A module's `api/`, `codes/`, `ports/` and root package are importable by
other modules and by other repositories; its `internal/` is not, and the
compiler enforces that.

A module has only the contract packages it needs. Rails offers nothing
inbound, so it has no `api/` or `codes/`; it implements the ledger's ports.

## Dependency rules

Enforced by `internal/archtest` on every `make test`. The checker finds
every module (a top-level folder with a `module.go`) and applies the same
rules to each, including that SQL and migrations name only the module's own
schema; it is itself tested against a good and a bad fixture repository.
Module dependencies are declared in its test (today only rails → ledger), so
a new edge is a visible decision.

```
application  → domain, application/port, persistence, <module>/ports, <module>/codes
               (persistence is the concrete repos: the deliberate "repos are
               concrete" decision, not a leak; use cases never see database/sql)
persistence  → domain
domain       → <module>/codes
adapter      → application, api, domain           (only package that sees both sides)
transport    → api, codes                         (never application or domain)
<module>/api → <module>/codes
codes, ports → nothing in the module
other module → <module>/api only (never LedgerAdmin)
implementer  → <module>/ports only
platform     → nothing outside platform
main         → module roots and platform
```

Nothing imports another module's `internal/`. No transaction crosses a module
boundary; consistency across modules is idempotency keys, outcome rows and
reconciliation.

## Modules

```
ledger   accounts, transactions, entries, balances, idempotency_keys; rails as an outbound port
rails    attempts and outcomes; implements ledger/ports (internal, rtp, card, swift);
         acts as the ledger's outbox for rail outcomes
wallet   planned: operations; depends on ledger/api.Ledger only, never on rails.
         Picks a rail by name on the ledger call (topup, transfer, pay, refund)
```

Planned modules (README roadmap): identity, limits, wallet, risk, disputes
(Build 2); settlement, lending, promotions (Build 3). Each gets the same shape
and depends on others only through their `api/`; nothing depends on a vertical.

## Contract surfaces

- `api.Ledger`: what products call. Accounts, balance, history, Post, Reverse,
  Capture, Void, GetTransaction. Other modules get it from `Module.API()`.
- `api.LedgerAdmin`: rebuild and verify balances, reconciliation. The module
  wires it to its own transport; `Module` has no accessor for it, so no other
  module can receive it.
- `codes`: every error code a caller can receive. Renaming one is a breaking change.
- `ports.Rail` / `ports.RailRegistry`: what the ledger needs from rails. The
  registry hands out due outcomes under a lease (`ClaimDue`), takes the
  ledger's ack (`MarkApplied`) and failures (`RecordFailure`), and pages
  attempts for reconciliation (`Attempts`).

## Background jobs

Jobs are application use cases (`job_*.go`). `module.go` schedules them via
`platform/scheduler`; `main` only calls Start/Stop. Every job must be safe on N
replicas: a lease or `SKIP LOCKED` held for the duration of the work, never
just the select.

- `outcomes`: two-phase rail outcomes (rail decides, ledger applies, ack)
- `expire-holds`: one transaction per internal hold
- `reconcile`: compares rails with the ledger every 15 minutes, logs findings

## Decisions

- one repository, modular monolith. Module contracts are importable, so other
  products can use the ledger as a library without a split. A module gets its
  own `go.mod` or repository when a second consumer needs a different release
  cadence, not before; the enforced boundaries make that step mechanical

- kind is product vocabulary (free text); lineage is ledger-owned
  (relation + parent_id); every child carries a relation (CHECK)
- three lifecycle states: pending | posted | voided. A hold settled through
  capture children ends voided; `domain.Summarize` labels it
  `settled_via_children` or `partially_settled`
- business rules live in Go, not SQL. SQL keeps integrity: append-only
  triggers, and the live-balance query the cache is verified against
- balances are a rebuildable cache, verified continuously
- outbound ports are exported because Go's `internal/` rule would otherwise
  block implementers
- rail-backed holds are resolved by their rail only: never by the expirer,
  never by an HTTP capture; an HTTP void asks the rail to cancel
- `rails.outcomes` records both the rail's decision and whether the ledger
  applied it (`applied_at`). Deliberate: rails is the outbox, the ledger acks
  through ports. Never join `ledger.*` and `rails.*` in SQL
- repos are concrete: tests run against real Postgres, so repo interfaces
  would have one implementation and no test boundary. The only application
  port is UnitOfWork, so use cases never see a database transaction
- one `Service` struct: it is a dependency bag; what a caller may reach is
  decided by the api interfaces, not by splitting the struct
- api is designed as if remote: every mutating transaction call takes a
  required idempotency key, errors cross as codes, DTOs are plain values.
  `CreateAccount` is the exception: it is idempotent on its natural key
  (owner, type, currency), and a repeat returns `ACCOUNT_EXISTS`
- migrations are append-only from the v0.1.0 baseline. Before it they were
  squashed into one `001_schema` per module; the history is in `docs/stories`
- tests run against their own database (`fintech_test`, `make test-db`) and
  packages run serially (`-p 1`) because they share it
- connection pool: a rail-backed post holds two connections (ledger tx +
  rails tx) until the outbox lands, and every replica opens its own pool, so
  replicas × DB_MAX_OPEN < max_connections − headroom (migrations, psql, admin)
- the outbox is the planned fix for initiating a rail inside the ledger's
  transaction: commit pending first, then initiate. It removes the second
  connection and the orphan window
- docs: this file and the code always describe the system as it is now, and
  every change updates them. `docs/stories` is a dated journal of why; a story
  is never rewritten to match later code
- deployment shapes: in-process today; later, `cmd/ledger-svc` with a gRPC
  adapter implementing the same api interfaces

## Parked

Deliberately not done yet. Each line: what, why it waits, where it lands.

- authentication (API keys for products, JWT for users): nothing is exposed yet; Build 2
- idempotency keys scoped per caller: needs authentication first; Build 2
- chargebacks (rail-initiated reversals): need the disputes lifecycle; Build 2
- incremental authorization (raise a hold before capture): additive, no consumer yet; Build 3
- merchant-chosen capture amount on card holds: card settles the full authorization today; Build 3
- outbox (call rails after the ledger commits): bigger change, reconciliation covers the gap; capstone, reusing dukan's `pkg/outbox`
- structured logging (`slog`), request ids, metrics, traces: the outcome worker is the showcase; capstone
- composition root as an `app` package (module interface, `/healthz`, `/readyz`, panic-recovery middleware): worth it when a second module serves HTTP; with the wallet
- a fresh schema per test (dukan's `testdb.Fresh`) or testcontainers: would remove `drain()` and `-p 1`; capstone
- reconciliation watermark (only attempts changed since the last clean run): a full scan is fine at this size; capstone
- statements as an async read model (CQRS): only if read load demands it; capstone
- a real status for holds settled through captures: the settlement label covers it; capstone
- `platform/` as its own repository: dukan depends on it too; when a third consumer appears
- `internal/archtest` as its own library: dukan carries a drifted copy; extract once both repos share one module layout, after evaluating `go-arch-lint` and `arch-go` (neither checks SQL schema ownership, which this one does)
- a clearer error than `INVALID_JSON` for a fractional amount: cosmetic

## Not this

- no re-exporting domain types as api types
- no transactions passed across modules
- no interfaces for structs with one implementation and no test boundary
- no product nouns inside the ledger
- no business rules in SQL
- no cross-schema SQL joins or foreign keys
- no admin surface handed to another module
