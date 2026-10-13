# Story 14: Hold expiry

**Goal:** an internal hold nobody captures or voids gives its money back.

Shipped in 30de809 without a story doc; written at v0.1.0 to describe the
behaviour as it stands, after the changes from 21a.

**Acceptance**

- a pending transaction gets `expires_at`, default now + 7 days unless the
  caller sets one
- the `expire-holds` job runs every minute and voids internal holds past
  `expires_at` with status_reason "expired", keyed expire:{id}
- a partially captured hold releases only what it still reserves
- two workers never void the same hold twice
- rail-backed holds are never expired by the clock: their rail settles,
  bounces or cancels them (21a)

**Design**

- one transaction per hold: `SELECT ... LIMIT 1 FOR UPDATE SKIP LOCKED`, then
  the void, in the same transaction. The lock covers the work, replicas
  partition the holds, and no transaction ever holds balance locks for more
  than one hold (a batch could deadlock with ordinary posts)
- `pkg/scheduler.Loop` runs jobs on a ticker with panic recovery; the module
  owns its jobs, main only starts and stops them

**History**

- the first version selected a batch with SKIP LOCKED in a transaction that
  committed immediately, so the lock was gone before the work started; the
  "partitioned across replicas" comment was false. Fixed in 21a
- a manual `POST /admin/holds/expire` existed and was removed; the job is the
  only path

**Note**

- rail-backed holds still get `expires_at` = settlement delay + 48h grace.
  Nothing acts on it since 21a; it records when the hold was expected to
  resolve, and an overdue one is visible to reconciliation as unapplied or stuck
