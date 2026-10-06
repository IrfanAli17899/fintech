# Story 19 — Settlement worker

**Goal:** deferred transactions finish: posted when the rail confirms, voided when it bounces.

**Acceptance**

- Settle(id) = full capture keyed settle:{id}; Bounce(id, reason) = void keyed bounce:{id}
- SettleDue asks RailRegistry.Due for attempts past settle_after, calls rail.Settle,
  applies the outcome; unknown/other rail errors are logged and retried next run
- a RailBounce from the rail voids the hold with status_reason "bounced: …"
- scheduler job "settle" every minute; POST /admin/settlements/run on demand
- two workers over 20 due attempts settle each exactly once
- a settled swift transfer is final: reversing its capture → RAIL_NOT_REVERSIBLE

**Design**

- the capture child carries the hold's rail and rail_ref; initiateRail skips
  transactions that already have a ref, so settlement never re-initiates
- RailBounce is an api type: the ledger must recognise it without importing rails
- status_reason records why a transaction is in its current status; set on void
  (expired, bounced, customer-supplied)
- orphan attempts (rail knows a transfer the ledger never committed) are logged
  and skipped, not fatal — the adapter tests produce them by design, and so
  would a real provider after a rolled-back post

**Decisions**

- order is rail.Settle then ledger.Settle. A crash between leaves an attempt
  settled while the hold later expires — money received but not credited.
  Reversing the order is worse (posting before knowing the outcome). The gap is
  closed by reconciliation (21): attempt state vs transaction state
- SKIP LOCKED partitions concurrent workers only while their SELECTs overlap;
  late duplicates fail cleanly at the rail's state transition
- per-attempt transactions, never a batch void/capture: one bad attempt fails alone
