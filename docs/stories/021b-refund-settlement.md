# Story 21b: Refund settlement

**Goal:** a settled rail refund keeps counting against the transaction it
refunds, and a voided hold says whether it was settled or cancelled.

**Acceptance**

- a refund outcome the rail settles posts the pending refund child in place
  (postPending): reservation released, entries applied as posted,
  status = posted, posted_at = now(); idempotent on refund-settle:{key}
- after a 4000 refund of a 5000 card payment settles, a further 1001 is
  REVERSAL_EXCEEDS_ORIGINAL and the remaining 1000 is accepted
- a settled refund child is posted and has no children
- Void on a pending refund child → RAIL_CANNOT_CANCEL: it is a rail.Reverse in
  flight, which no rail can withdraw; it settles or is refused
- transaction_summaries.settlement: settled_via_children (voided, captures =
  total), partially_settled (voided, 0 < captures < total), otherwise the status
- history entries carry posted_at next to created_at; order stays on seq

**Design**

- option (b) from the review: every hold still settles through capture
  children (voided hold + posted child). Only refund children post in place,
  because they never have children of their own; posting a parent that has
  capture children would count its entries twice in the live-balance CTE and
  the summaries view
- refunds were settled by Capture before this, which marked the child voided
  and put the money on a grandchild; ChildAmountsByAccount (posted + pending
  children) then counted the settled refund as zero
- "voided means fully captured" stays a wart of the hold model; the
  settlement label makes it readable without a fourth status. Its own story later
- history sorts on seq, the stable cursor. A settled card topup appears at its
  authorization time; posted_at says when it settled. Re-sorting by posted_at
  would break pagination

**Cutover**

- before 21a (c33ec3f), rail refunds posted directly; they are posted children
  and count correctly
- between 21a and 21b, settled rail refunds are voided children with a posted
  capture grandchild, and do not count toward their original's total. These
  exist only in development databases (22 locally); `make db-reset` clears
  them, and 21c reports them
- from 21b on, a settled refund is a posted child
