# Architecture

One Go module, one Postgres, one schema per bounded-context module.
Modules talk only through exported contract packages; implementation is
hidden under `internal/` and enforced by the compiler.

## Status

Target layout. Migration in progress:

- done: schema per module, own migration tracking table, no cross-schema FKs,
  rails depends on ledger contract only, ports/ split out of api/, layer
  renames services → application, repositories → persistence,
  handlers → transport/http (40e06ec); two-phase rail outcomes with
  leases, backoff, Cancel and pending refunds (21a); refund children settle
  in place, settlement label on summaries (21b)
- pending (21c): reconciler
- pending (commit 2): one file per operation, single Service, UnitOfWork
- pending (commit 3 / story 20): codes/ split out, api.Ledger, api.LedgerAdmin,
  adapter/, module.API()/Admin(), wallet module

Anything listed as pending is not yet true in the code.

## Layout

```
cmd/server/main.go               composition root: migrate, wire, routes, jobs, shutdown

internal/<module>/
  api/                           inbound contract: what the module offers; consumers import
  ports/                         outbound contracts: what the module needs; implementers import
  codes/                         error code strings; leaf, zero deps
  migrations/  migrate.go        schema <module>.*, embedded, own tracking table
  module.go                      New(deps) *Module; API(); Admin(); RegisterRoutes; Start/Stop; Seed
  internal/
    domain/                      entities, invariants, views, error constructors
    application/                 use cases (one file per operation), background jobs
                                 (settle, expire, reconcile), port/ (UnitOfWork only)
    persistence/                 Postgres repos, concrete types
    adapter/                     api.<Module> and api.<Module>Admin over application;
                                 domain <-> api mapping
    transport/http/              handlers + DTOs; depend on api only

pkg/                             dependency-free infrastructure: database, errdefs, httpx, scheduler
```

## Dependency rules

```
application  → domain, application/port, persistence, <module>/ports, <module>/codes
               (persistence here is the concrete repos; this is the deliberate
               "repos are concrete" decision, not a leak. Only UnitOfWork is abstracted.)
persistence  → domain
adapter      → application, api, domain           (only package that sees both sides)
transport    → api                                (never application)
<module>/api → <module>/codes
other module → <module>/api only (never Admin)
implementer  → <module>/ports only
main         → module roots only
```

Nothing imports another module's internal/. No transaction crosses a module
boundary; consistency across modules is idempotency keys + intent rows + reconciliation.

## Modules

```
ledger   accounts, transactions, entries, balances, idempotency_keys; rails as an outbound port
rails    attempts; implements ledger/ports.Rail (internal, rtp, card, swift);
         acts as the ledger's outbox for rail outcomes
wallet   operations; depends on ledger/api.Ledger only, never on rails.
         Picks a rail by name on the ledger call (topup, transfer, pay, refund)
```

## Contract surfaces

- api.Ledger: what products call. Post, Reverse, Capture, Void, reads.
- api.LedgerAdmin: rebuild/verify balances, reconciliation report. Wired
  by main to transport only; no module receives it.
- ports.Rail / ports.RailRegistry: what the ledger needs from rails,
  including the ack that a rail outcome was applied (MarkApplied).

## Background jobs

Jobs are application use cases. module.go owns their scheduling via
pkg/scheduler and starts/stops them; main only calls Start/Stop.
Every job must be safe on N replicas: a lease or SKIP LOCKED held for the
duration of the work, never just the select.

## Decisions

- kind is product vocabulary (free text); lineage is ledger-owned (relation + parent_id);
  every child carries a relation (CHECK, migration 013)
- three lifecycle states: pending | posted | voided; everything else derived
- balances are a rebuildable cache, verified continuously
- outbound ports are exported because Go's internal/ rule would otherwise block implementers
- rail-backed holds are resolved by their rail only: never by the expirer, never by HTTP void/capture
- rails.attempts records both the rail's decision and whether the ledger applied it
  (applied_at). This is deliberate: rails is the outbox, the ledger acks through
  ports. Never join ledger.* and rails.* in SQL.
- repos are concrete (chosen): tests run against real Postgres, so repo
  interfaces would have one implementation and no test boundary. The only
  application port is UnitOfWork. Revisit if in-memory fakes are added.
- api is designed as if remote: every mutating call takes a required idempotency
  key, errors cross as codes only, DTOs are plain values (no pointers to internals,
  no iterators, no driver types)
- migrations are append-only (since story 15); fixes are new migrations
- connection pool: a rail-backed post holds two connections (ledger tx +
  rails tx) until the outbox lands, and every replica opens its own pool, so
  replicas × DB_MAX_OPEN < max_connections − headroom (migrations, psql, admin)
- deployment shapes: in-process today; Build 4 adds cmd/ledger-svc and cmd/finance-svc
  with gRPC adapters implementing the same api interfaces, plus an outbox

## Not this

- no re-exporting domain types as api types
- no transactions passed across modules
- no interfaces for structs with one implementation and no test boundary
- no product nouns inside the ledger
- no cross-schema SQL joins or foreign keys
- no Admin surface handed to another module
