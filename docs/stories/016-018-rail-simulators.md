# Stories 16–18 — Rail simulators

**Goal:** one rail per behaviour pattern, so the ledger's lifecycle handling is
exercised against instant, deferred-reversible, and deferred-bounceable money.

## 16 — RTP (Raast, SARIE)

- instant, irreversible, single-currency; registered twice with different currencies
- Initiate → immediate; Reverse refused at the rail and by the ledger
- pattern: domestic real-time payments anywhere (Raast, SARIE, UPI, PIX, FedNow)

## 17 — Card

- deferred (48h), reversible, no bounce; Initiate records an attempt in
  rails.attempts and returns deferred, so the ledger posts a pending hold
- Settle: initiated → settled; Reverse (chargeback): settled → reversed;
  wrong-state transitions are refused
- first rails-owned state: schema rails, own migrations and tracking table
- known gap: Initiate writes on the pool, not the ledger's tx, so a rolled-back
  post leaves an orphan attempt — exactly what a real provider would, and what
  the reconciler (21) matches on

## 18 — SWIFT

- deferred (72h), never reversible, can bounce; amounts ending in 99 are
  rejected at settlement so tests can drive the bounce path deterministically
- Settle returns ErrBounced and moves the attempt to bounced; the ledger (19)
  turns that into a void with a reason
- pattern: correspondent banking and any rail where failure arrives late

**Decisions**

- rails are patterns, not geographies: mada is the card pattern, SARIE is the
  RTP pattern. New markets are registry entries, not new code
- rails owns its state (rail_attempts) in its own migration dir and never
  references ledger tables; transaction_id is an opaque correlation id. This is
  what lets rails become a service without a schema split
- rail-level tests live in the rails module; the ledger tests only check the
  resulting lifecycle
