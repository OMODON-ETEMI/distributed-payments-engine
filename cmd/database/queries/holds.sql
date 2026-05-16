-- Funds holds (reservation) queries

-- name: CreateHold :one
INSERT INTO funds_holds (
  account_id, transfer_request_id, journal_transaction_id, idempotency_key_id, status, currency_code, amount, remaining_amount, released_amount, captured_amount, reason_code, reason, expires_at
)
VALUES (@account_id::uuid, @transfer_request_id::uuid, @journal_transaction_id::uuid, @idempotency_key_id::uuid, @status::hold_status, @currency_code::char(3), @amount::numeric(20,8), @remaining_amount::numeric(20,8), @released_amount::numeric(20,8), @captured_amount::numeric(20,8), @reason_code, @reason, @expires_at::timestamptz)
RETURNING *;

-- name: GetActiveHoldsForAccount :many
SELECT * FROM funds_holds WHERE account_id = @account_id::uuid AND status = 'active' ORDER BY created_at DESC;

-- name: ReleaseHold :one
UPDATE funds_holds
SET status = 'released', released_at = now(), released_amount = released_amount + @amount::numeric(20,8), remaining_amount = remaining_amount - @amount::numeric(20,8)
WHERE id = @id::uuid AND status = 'active' AND remaining_amount >= @amount::numeric(20,8)
RETURNING *;

-- name: ConsumeHold :one
UPDATE funds_holds
SET status = 'consumed', captured_at = now(), captured_amount = captured_amount + @amount::numeric(20,8), remaining_amount = remaining_amount - @amount::numeric(20,8)
WHERE id = @id::uuid AND status = 'active' AND remaining_amount >= @amount::numeric(20,8)
RETURNING *;

-- name: GetActiveHoldByTransferRequestID :one
SELECT * FROM funds_holds
WHERE transfer_request_id = @transfer_request_id::uuid
  AND status = 'active'
LIMIT 1
FOR UPDATE;
