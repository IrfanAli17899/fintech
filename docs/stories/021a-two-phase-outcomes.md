# Story 21a: Two-phase rail outcomes

**Goal:** a rail decision and its ledger effect can never silently disagree.
The rail decides, the ledger applies, then acks; a crash between them is
finished by the next run.

**Acceptance**

- rails.outcomes holds every decision (settle, cancel, refund) with applied_at;
  rails is the ledger's outbox, the ledger acks through ports.MarkApplied
- one ports.Decision (undecided | settled | bounced | cancelled), never flags
- ApplyOutcomes leases its whole batch: lease = batch × 10s rail budget + 30s;
  job batches of 20; returns Claimed and loops on it
- phase-1 failure → RecordFailure: attempt_count++, backoff 1, 2, 4 ... minutes
  capped at 6h, lease released; ClaimDue skips rows still backing off
- every rails write is one tx under the attempt's row lock: Initiate (attempt +
  settle outcome), Settle, Cancel (cancel outcome + supersede settle + attempt),
  Reverse (refund outcome)
- concurrent Settle and Cancel: exactly one wins; the loser gets ErrWrongState
  or ErrRailAlreadyDecided, never a half-written pair
- Void on a card hold: key committed, then rail.Cancel outside the lock; a
  cancel that loses to settlement → HOLD_ALREADY_SETTLING (409)
- Void on SWIFT → RAIL_CANNOT_CANCEL, checked before the key is linked, so the
  key is never spent
- Reverse on a rail-backed transaction posts a pending refund child, commits,
  then calls rail.Reverse keyed on the child id; partial refunds allowed
- a replay of a still-pending rail void or refund asks the rail again
- expirer: one tx per hold (LIMIT 1 FOR UPDATE SKIP LOCKED, then void in the
  same tx), so locks never pile up across holds and replicas partition the work
- 15s per-request context timeout; DB_MAX_OPEN (default 25)

**Design**

- migration 003 backfills history (settled/bounced/reversed attempts as
  applied settle outcomes) and every in-flight attempt as an undecided settle
  outcome; without the second insert in-flight attempts would never come due.
  Locally it backfilled 39, not the 37 expected: two attempts were written by
  a test run against the old non-atomic Initiate, whose outcome insert failed
  after the attempt insert succeeded. The bug proving itself
- a cancel supersedes the settle outcome by deciding it cancelled; the worker
  acks it as Superseded and the cancel outcome does the void
- tests migrate under sync.Once; `make test` runs packages with -p 1 because
  they share one database and the worker claims everything due

**Expected after deploying**

- the first worker runs log DIVERGENCE for holds voided over HTTP before the
  rail guards (72a1016) whose attempts are still live, and for orphan attempts
  from rolled-back posts. Both are real findings, not regressions; 21c reports
  them as divergence and orphan_attempt
- the 15s request timeout widens the orphan window: rails' Initiate tx commits
  before the ledger tx, so a timeout between them leaves an attempt with no
  ledger transaction. The worker settles it, finds NotFound, logs and acks.
  The outbox (capstone) closes this by initiating after the ledger commits

**Not in 21a**

- refunds still settle by capturing the pending child, which marks the child
  voided and drops it from ChildAmountsByAccount, so a settled refund stops
  counting toward the original's total. 21b settles refund children in place
  (PostPending)
- reconciler and the stuck (attempt_count ≥ 10) report: 21c
