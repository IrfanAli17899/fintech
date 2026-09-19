# Story 8 — No overdraft under concurrency

**Goal:** a liability account can never go negative, however many transfers hit it at once.

**Acceptance**

- liability debit exceeding available → 400 INSUFFICIENT_FUNDS
- asset/expense/revenue debits are not checked (our money, may go negative)
- 20 concurrent 100-unit transfers from a 1000 wallet → exactly 10 succeed
- A→B and B→A transfers running simultaneously never deadlock
- balance cache stays consistent under concurrency (verify clean)

**Design**

- pessimistic: SELECT ... FOR UPDATE on balances rows, inside the posting tx,
  before any write; released at commit or on crash
- lock every touched account, ordered by id — deterministic order is what
  prevents deadlocks (A→B and B→A both lock min(A,B) first)
- every account gets a zero balances row at creation so there is always a row to lock
- check available = NormalBalance(locked sums) ≥ this tx's debits per account

**Decisions**

- row locks over SERIALIZABLE: predictable queueing on a hot wallet instead of
  abort-and-retry storms
- lock balances not accounts: the balance row is what the decision reads
- only liability debits are checked; asset/revenue accounts are never
