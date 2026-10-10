-- A hold settled through children is voided with captures summing to its total;
-- "voided" alone cannot tell that from a cancelled hold. settlement says which.
CREATE OR REPLACE VIEW ledger.transaction_summaries AS
SELECT
  s.id, s.total, s.captured, s.reversed,
  CASE
    WHEN s.status = 'voided' AND s.total > 0 AND s.captured >= s.total THEN 'settled_via_children'
    WHEN s.status = 'voided' AND s.captured > 0 THEN 'partially_settled'
    ELSE s.status::text
  END AS settlement
FROM (
  SELECT
    t.id, t.status,
    COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'debit'), 0) AS total,
    COALESCE((
      SELECT SUM(ce.amount)
      FROM ledger.transactions c
      JOIN ledger.entries ce ON ce.transaction_id = c.id
      WHERE c.parent_id = t.id AND c.relation = 'captures' AND c.status = 'posted'
        AND ce.direction = 'debit'
    ), 0) AS captured,
    COALESCE((
      SELECT SUM(re.amount)
      FROM ledger.transactions r
      JOIN ledger.entries re ON re.transaction_id = r.id
      WHERE r.parent_id = t.id AND r.relation = 'reverses' AND r.status = 'posted'
        AND re.direction = 'credit'
    ), 0) AS reversed
  FROM ledger.transactions t
  JOIN ledger.entries e ON e.transaction_id = t.id
  GROUP BY t.id, t.status
) s;
