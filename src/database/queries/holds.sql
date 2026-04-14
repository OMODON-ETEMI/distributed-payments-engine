-- Funds holds (reservation) queries

-- name: CreateHold :one
INSERT INTO funds_holds (
  account_id, transfer_request_id, journal_transaction_id, idempotency_key_id, status, currency_code, amount, remaining_amount, released_amount, captured_amount, reason_code, reason, expires_at
)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::hold_status, $6::char(3), $7::numeric(20,8), $8::numeric(20,8), $9::numeric(20,8), $10::numeric(20,8), $11, $12, $13::timestamptz)
RETURNING *;

-- name: GetActiveHoldsForAccount :many
SELECT * FROM funds_holds WHERE account_id = $1::uuid AND status = 'active' ORDER BY created_at DESC;

-- name: ReleaseHold :one
UPDATE funds_holds
SET status = 'released', released_at = now(), released_amount = released_amount + $2::numeric(20,8), remaining_amount = remaining_amount - $2::numeric(20,8)
WHERE id = $1::uuid AND status = 'active' AND remaining_amount >= $2::numeric(20,8)
RETURNING *;

-- name: ConsumeHold :one
UPDATE funds_holds
SET status = 'consumed', captured_at = now(), captured_amount = captured_amount + $2::numeric(20,8), remaining_amount = remaining_amount - $2::numeric(20,8)
WHERE id = $1::uuid AND status = 'active' AND remaining_amount >= $2::numeric(20,8)
RETURNING *;
