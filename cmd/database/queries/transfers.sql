-- Transfer request queries

-- name: CreateTransferRequest :one
INSERT INTO transfer_requests (
  idempotency_key_id, customer_id, source_account_id, destination_account_id, currency_code, amount, fee_amount, client_reference, external_reference, metadata
)
VALUES (@idempotency_key_id::uuid, @customer_id::uuid, @source_account_id::uuid, @destination_account_id::uuid, @currency_code::char(3), @amount::numeric(20,8), @fee_amount::numeric(20,8), @client_reference, @external_reference, @metadata::jsonb)
RETURNING *;

-- name: GetTransferRequestByID :one
SELECT * FROM transfer_requests WHERE id = @id::uuid LIMIT 1;

-- name: GetTransferRequestByIdempotencyKey :one
SELECT * FROM transfer_requests WHERE idempotency_key_id = @idempotency_key_id::uuid LIMIT 1;

-- name: ListTransferRequestsByCustomer :many
SELECT * FROM transfer_requests WHERE customer_id = @customer_id::uuid ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: UpdateTransferRequestStatus :one
UPDATE transfer_requests
SET status = @status::transfer_request_status,
    posted_at = CASE WHEN @status::transfer_request_status = 'posted' THEN now() ELSE posted_at END
WHERE id = @id::uuid
RETURNING *;

-- name: UpdateTransferRequestExternalRef :one
UPDATE transfer_requests
SET external_reference = @external_reference
WHERE id = @id::uuid
RETURNING *;