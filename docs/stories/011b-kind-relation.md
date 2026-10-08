# Story 11b — kind becomes product-owned

**Goal:** the ledger carries no business nouns; products name their own events.

**Change**

- transactions.kind: enum → free TEXT NOT NULL, validated non-empty only
- transactions.relation: ledger-owned enum captures | reverses, nullable,
  must have a parent when set
- ChildAmountsByAccount and the summaries view key on relation, not kind
- Reverse takes the child's kind from the caller; Capture inherits the parent's

**Why**

- topup/payment/fee are wallet words; a hospital posts pharmacy_sale with the
  same ledger. The library goal requires zero product vocabulary inside
- same split as TigerBeetle (code vs flags) and Modern Treasury (metadata vs
  reversal linkage): structure is the ledger's, meaning is the product's
