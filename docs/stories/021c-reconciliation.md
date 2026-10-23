# Story 21c: Reconciliation

**Goal:** every way the rail and the ledger can disagree is found and named,
without either side changing the other.

**Acceptance**

- Reconcile(at) pages every attempt with its outcomes through
  ports.RailRegistry.Attempts, loads the ledger side by id, and compares in
  Go. No SQL joins ledger.* to rails.*
- findings by kind:
  - orphan_attempt: the rail holds a transfer with no ledger transaction
  - divergence, with a reason: voided_but_live, settled_not_credited,
    settled_but_pending, bounced_but_credited, bounced_but_pending,
    cancelled_but_credited, cancelled_but_pending, refund_not_posted,
    refund_not_voided, refund_orphan, refund_never_sent
  - unapplied: decided, never applied, older than the grace window
  - stuck: attempt_count ≥ 10 (still retried), with last_error
  - trial_balance: within a currency, posted or pending debits ≠ credits.
    Checked per currency because summed across currencies a PKR imbalance
    and an opposite SAR imbalance cancel out; the report also carries the
    sums by currency and account type (added at the v0.1.0 audit; the first
    version summed everything together)
  - balance_drift: the cache differs from entries (same check as verify)
- anything younger than 10 minutes is skipped: a post whose rails tx has
  committed but whose ledger tx has not, or an outcome inside its lease
- GET /admin/reconciliation returns the report with counts per kind:reason;
  the "reconcile" job runs every 15 minutes and logs up to 50 findings
- each finding kind has a test that creates exactly that inconsistency, and a
  test that settled, refunded, cancelled and bounced flows report nothing

**Design**

- reconciliation reports, it never repairs. Repair is a deliberate act; the
  only repair so far is rails migration 004, below
- orphans are their own kind, not a divergence: their usual cause is the
  known Initiate-before-commit gap (see 21a), which the outbox closes

**Found on first run (local database)**

- 3 SWIFT bounces from story 19 development, decided at the rail and never
  applied by the old one-step worker. Migration 003 had marked all history
  applied on the assumption that the old worker always applied what it
  decided, so the new worker would never have touched them: holds pending
  forever, since the expirer skips rail-backed holds
- fix, rails migration 004: un-ack every decided settle outcome so the worker
  applies each once more. Safe for all history because applying is
  idempotent (settle:{id} and bounce:{id} replay); only the three changed
  anything. They are now voided with "bounced: rejected by correspondent bank"
- two hardenings from the same investigation: a phase-2 (apply) error now
  backs off through RecordFailure like a phase-1 error instead of aborting the
  batch; refund children post under refund-post:{key}, because 21a settled
  refunds by Capture under refund-settle:{key} and reusing that key for a
  different request would fail forever
- everything else reported locally was test data by design: orphans from the
  rail adapter tests (random transaction ids), the 22 refund_not_posted from
  the 21a → 21b cutover, and the rows the reconcile tests create. Tests now
  run against their own database (fintech_test, `make test-db`), so the dev
  report only shows the dev database; the ledger is append-only, so test rows
  could never have been cleaned up in place

**Not this**

- reconciliation is a full scan, paged by ref. Fine at this size; at scale it
  needs a watermark (only attempts changed since the last clean run)
