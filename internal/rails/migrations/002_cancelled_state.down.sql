-- Postgres cannot remove an enum value. 'cancelled' stays; nothing references
-- it after 003 is rolled back, so this is harmless.
SELECT 1;