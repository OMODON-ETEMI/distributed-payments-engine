-- balance projections & helpers
-- Queries to compute, read and upsert balance_projections reliably.

-- name: GetBalancesForAccount :many
SELECT
  account_id,
  currency_code,
  balance_kind,
  ledger_balance,
  available_balance,
  held_balance,
  version,
  computed_at
FROM balance_projections
WHERE account_id = $1::uuid
ORDER BY currency_code ASC, balance_kind ASC;

-- name: GetBalanceProjection :one
SELECT
  id,
  account_id,
  currency_code,
  balance_kind,
  ledger_balance,
  available_balance,
  held_balance,
  last_transaction_id,
  last_line_id,
  version,
  computed_at,
  created_at,
  updated_at
FROM balance_projections
WHERE account_id = $1::uuid
  AND currency_code = $2::char(3)
  AND balance_kind = $3
LIMIT 1;

-- name: ComputeLedgerBalance :one
SELECT
  COALESCE(SUM(CASE WHEN jl.side = a.ledger_normal_side THEN jl.amount ELSE -jl.amount END), 0)::numeric(20,8) AS ledger_balance,
  (SELECT id FROM journal_lines WHERE account_id = $1::uuid AND currency_code = $2::char(3) AND balance_kind = 'ledger' ORDER BY created_at DESC LIMIT 1) AS last_line_id,
  (SELECT jt.id FROM journal_transactions jt JOIN journal_lines jl2 ON jl2.journal_transaction_id = jt.id WHERE jl2.account_id = $1::uuid AND jl2.currency_code = $2::char(3) AND jt.status = 'posted' ORDER BY jt.posted_at DESC NULLS LAST, jt.created_at DESC LIMIT 1) AS last_transaction_id
FROM journal_lines jl
JOIN journal_transactions jt ON jt.id = jl.journal_transaction_id AND jt.status = 'posted'
JOIN accounts a ON a.id = jl.account_id
WHERE jl.account_id = $1::uuid
  AND jl.currency_code = $2::char(3)
  AND jl.balance_kind = 'ledger';

-- name: ComputeHeldAmount :one
SELECT COALESCE(SUM(remaining_amount), 0)::numeric(20,8) AS held_balance
FROM funds_holds
WHERE account_id = $1::uuid
  AND currency_code = $2::char(3)
  AND status = 'active';

-- name: RebuildBalanceProjection :exec
WITH ledger AS (
  SELECT COALESCE(SUM(CASE WHEN jl.side = a.ledger_normal_side THEN jl.amount ELSE -jl.amount END), 0)::numeric(20,8) AS ledger_balance
  FROM journal_lines jl
  JOIN journal_transactions jt ON jt.id = jl.journal_transaction_id AND jt.status = 'posted'
  JOIN accounts a ON a.id = jl.account_id
  WHERE jl.account_id = $1::uuid
    AND jl.currency_code = $2::char(3)
    AND jl.balance_kind = 'ledger'
), held AS (
  SELECT COALESCE(SUM(remaining_amount), 0)::numeric(20,8) AS held_balance
  FROM funds_holds
  WHERE account_id = $1::uuid
    AND currency_code = $2::char(3)
    AND status = 'active'
), lasts AS (
  SELECT
    (SELECT jt.id FROM journal_transactions jt JOIN journal_lines jl2 ON jl2.journal_transaction_id = jt.id WHERE jl2.account_id = $1::uuid AND jl2.currency_code = $2::char(3) AND jt.status = 'posted' ORDER BY jt.posted_at DESC NULLS LAST, jt.created_at DESC LIMIT 1) AS last_transaction_id,
    (SELECT id FROM journal_lines WHERE account_id = $1::uuid AND currency_code = $2::char(3) AND balance_kind = 'ledger' ORDER BY created_at DESC LIMIT 1) AS last_line_id
)
INSERT INTO balance_projections (
  account_id, currency_code, balance_kind, ledger_balance, available_balance, held_balance, last_transaction_id, last_line_id, version, computed_at
)
SELECT
  $1::uuid,
  $2::char(3),
  'ledger',
  ledger.ledger_balance,
  ledger.ledger_balance - held.held_balance,
  held.held_balance,
  lasts.last_transaction_id,
  lasts.last_line_id,
  COALESCE((SELECT version FROM balance_projections WHERE account_id = $1::uuid AND currency_code = $2::char(3) AND balance_kind = 'ledger'), 0) + 1,
  now()
FROM ledger, held, lasts
ON CONFLICT (account_id, currency_code, balance_kind)
DO UPDATE SET
  ledger_balance = EXCLUDED.ledger_balance,
  available_balance = EXCLUDED.available_balance,
  held_balance = EXCLUDED.held_balance,
  last_transaction_id = EXCLUDED.last_transaction_id,
  last_line_id = EXCLUDED.last_line_id,
  version = balance_projections.version + 1,
  computed_at = now();

-- name: UpsertBalanceProjectionWithExpectedVersion :exec
-- Params: account_id, currency_code, balance_kind, ledger_balance, available_balance, held_balance, last_tx_id, last_line_id, expected_version
INSERT INTO balance_projections (
  account_id, currency_code, balance_kind, ledger_balance, available_balance, held_balance, last_transaction_id, last_line_id, version, computed_at
)
VALUES ($1::uuid, $2::char(3), $3, $4::numeric(20,8), $5::numeric(20,8), $6::numeric(20,8), $7::uuid, $8::uuid, 1, now())
ON CONFLICT (account_id, currency_code, balance_kind)
DO UPDATE SET
  ledger_balance = EXCLUDED.ledger_balance,
  available_balance = EXCLUDED.available_balance,
  held_balance = EXCLUDED.held_balance,
  last_transaction_id = EXCLUDED.last_transaction_id,
  last_line_id = EXCLUDED.last_line_id,
  version = balance_projections.version + 1,
  computed_at = now()
WHERE balance_projections.version = $9;
