# fintech

A double-entry ledger in Go, built as a modular monolith: money moves through
balanced, append-only transactions, and external payment rails (simulated
card, SWIFT and instant bank transfers) sit behind a port the ledger owns.

> **Not production software.** There is no authentication, the payment rails
> are simulators, and there are no metrics yet. It is a study in getting the
> money path right: idempotency, holds, refunds, and keeping a ledger and an
> external rail from silently disagreeing.

## What it does

- **Double-entry, append-only.** Every transaction balances; entries can never
  change and transactions can never be deleted (enforced by database triggers).
- **Idempotent writes.** Every write takes an idempotency key; a retry returns
  the original result, a reused key with a different request is rejected.
- **Holds.** Reserve funds, capture all or part, void the rest, or let them
  expire. Available balance accounts for outgoing holds.
- **Reversals.** Full or partial refunds, never past the original's total.
- **Rails.** Internal transfers, instant irreversible rails (Raast, SARIE),
  card (authorize, settle later, cancel, refund) and SWIFT (slow, can bounce).
- **Two-phase rail outcomes.** The rail decides, the ledger applies, then
  acknowledges. A crash between the two is finished by the next run.
- **Reconciliation.** A report and a scheduled job compare every rail attempt
  with the ledger and name each kind of disagreement.

## Architecture

```mermaid
flowchart LR
  main["cmd/server"] --> ledger
  main --> rails

  subgraph ledger["internal/ledger"]
    direction TB
    transport["transport/http"] --> api["api + codes"]
    adapter --> api
    adapter --> application
    application --> domain
    application --> persistence
    application --> ports
  end

  subgraph rails["internal/rails"]
    railimpl["card · swift · rtp · internal"]
  end

  railimpl -. implements .-> ports
  persistence --> pg[("Postgres<br/>ledger.* · rails.*")]
  railimpl --> pg
```

- Each module owns a Postgres schema; no SQL ever joins across them.
- Other modules use the ledger only through `api` (`Module.API()`); rails
  implement the ledger's `ports`. Everything else is under `internal/`.
- The dependency rules are tested: `internal/archtest` fails the build when a
  layer reaches somewhere it should not.

Read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the rules and the
decisions behind them, and [docs/stories](docs/stories) for how it was built,
one story at a time. The best single read is the "Found on first run" section
of [story 21c](docs/stories/021c-reconciliation.md): the reconciler's first run
found a real bug in the migration written two stories earlier.

## Run it

Requires Go 1.26 and Docker.

```bash
make up      # Postgres in Docker
make run     # migrates, seeds system accounts, serves on :4000
```

```bash
# open two accounts
curl -s localhost:4000/accounts -d '{"owner_type":"system","owner_id":"cash","currency":"PKR","type":"asset"}'
curl -s localhost:4000/accounts -d '{"owner_type":"customer","owner_id":"ali","currency":"PKR","type":"liability"}'

# top up Ali by card: comes back pending until the card rail settles it
curl -s localhost:4000/transactions -d '{
  "kind": "topup", "rail": "card", "idempotency_key": "topup-1",
  "entries": [
    {"account_id": "<cash id>", "amount": 5000, "direction": "debit"},
    {"account_id": "<ali id>",  "amount": 5000, "direction": "credit"}
  ]}'

curl -s localhost:4000/accounts/<ali id>/balance
curl -s localhost:4000/admin/reconciliation
```

| Endpoint | |
|---|---|
| `POST /accounts`, `GET /accounts?owner_type=&owner_id=`, `GET /accounts/{id}` | accounts |
| `GET /accounts/{id}/balance`, `GET /accounts/{id}/transactions` | balance, statement |
| `POST /transactions`, `GET /transactions/{id}` | post, read |
| `POST /transactions/{id}/reverse` · `/capture` · `/void` | refund, settle a hold, release a hold |
| `GET /admin/balances/verify`, `POST /admin/balances/rebuild` | balance cache |
| `GET /admin/reconciliation` | rail vs ledger report |

Amounts are integers in minor units.

## Test

```bash
make test
```

Tests run against a real Postgres in their own database (`fintech_test`,
created on first run), with the race detector on.

## Layout

```
cmd/server/            composition root
internal/ledger/       the ledger module: api, codes, ports, migrations, internal/
internal/rails/        rail simulators implementing the ledger's ports
internal/archtest/     dependency-rule tests
pkg/                   database, errors, http helpers, scheduler
docs/                  architecture and story-by-story decisions
```
