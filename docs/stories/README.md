# Build journal

The ledger was built one story at a time. Each story records what was decided
and why, **as the code was at the time**. File names, endpoints and migration
numbers in older stories may no longer exist; that is the point of a journal.

For how the system works now, read [ARCHITECTURE.md](../ARCHITECTURE.md) and
the code. These stories are for why it ended up that way.

Migration numbers refer to the chain before v0.1.0, which was squashed into
one baseline per module in `d670f82`. The old files are in git history.

## Start here

Five stories carry most of the lessons:

- [004 Append-only ledger](004-append-only.md): the database itself refuses to rewrite history
- [007 Idempotent posting](007-idempotency.md): a retried request never moves money twice
- [008 No overdraft under concurrency](008-overdraft.md): row locks in a fixed order, and why not SERIALIZABLE
- [021a Two-phase rail outcomes](021a-two-phase-outcomes.md): the rail decides, the ledger applies, a crash in between is finished later
- [021c Reconciliation](021c-reconciliation.md): its first run found a real bug in a migration written two stories earlier

## All stories

| # | Story | Goal |
|---|---|---|
| 1 | [Accounts](001-accounts.md) | typed, per-currency buckets every entry lands in |
| 2 | [Post a transaction](002-post-transaction.md) | record N entries atomically; reject anything that doesn't balance |
| 3 | [Integer minor units](003-integer-amounts.md) | money is never a float |
| 4 | [Append-only ledger](004-append-only.md) | the database refuses to rewrite history |
| 5 | [Balance from entries](005-balance.md) | balance is derived, never stored |
| 6 | [Balance cache](006-balance-cache.md) | fast balance reads, provably derived from entries |
| 7 | [Idempotent posting](007-idempotency.md) | a retried request never creates a second transaction |
| 8 | [No overdraft under concurrency](008-overdraft.md) | a liability account never goes negative, however many transfers hit it |
| 9–10 | [Reversals](009-010-reversals.md) | undo all or part of a posted transaction with a new, linked one |
| 11 | [Lifecycle states](011-lifecycle.md) | a transaction can hold funds before it moves them |
| 11b | [Kind becomes product-owned](011b-kind-relation.md) | the ledger carries no business nouns |
| 12 | [Available balance](012-available-balance.md) | holds reserve funds; promised funds are not spendable |
| 13 | [Capture and void](013-capture-void.md) | resolve a hold into money or release it |
| 14 | [Hold expiry](014-hold-expiry.md) | an abandoned internal hold gives its money back |
| 15 | [The Rail port](015-rail-interface.md) | move money through the outside world without knowing the provider |
| 16–18 | [Rail simulators](016-018-rail-simulators.md) | one rail per behaviour pattern |
| 19 | [Settlement worker](019-settlement.md) | deferred transactions finish: posted or bounced |
| 20 | [The ledger's contract](020-ledger-contract.md) | other modules use the ledger without seeing how it works |
| 21a | [Two-phase rail outcomes](021a-two-phase-outcomes.md) | a rail decision and its ledger effect never silently disagree |
| 21b | [Refund settlement](021b-refund-settlement.md) | a settled refund keeps counting against its original |
| 21c | [Reconciliation](021c-reconciliation.md) | every way the rail and the ledger can disagree is found and named |
