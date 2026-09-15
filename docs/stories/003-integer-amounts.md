# Story 3 — Integer minor units

**Goal:** money is never a float.

**Acceptance**

- amount is int64 minor units (paisa), BIGINT in Postgres
- amount > 0; amount ≤ 1e15 per entry

**Decisions**

- per-entry cap exists so summing entries can't overflow int64
  (~9,200 max-size entries needed to wrap)
- currency scale (2 for PKR) is not modelled yet; all amounts assumed minor units
