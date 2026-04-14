-- Transfer request queries

-- name: CreateTransferRequest :one
INSERT INTO transfer_requests (
  idempotency_key_id, customer_id, source_account_id, destination_account_id, currency_code, amount, fee_amount, client_reference, external_reference, metadata
)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::char(3), $6::numeric(20,8), $7::numeric(20,8), $8, $9, $10::jsonb)
RETURNING *;

-- name: GetTransferRequestByID :one
SELECT * FROM transfer_requests WHERE id = $1::uuid LIMIT 1;

-- name: GetTransferRequestByIdempotencyKey :one
SELECT * FROM transfer_requests WHERE idempotency_key_id = $1::uuid LIMIT 1;

-- name: ListTransferRequestsByCustomer :many
SELECT * FROM transfer_requests WHERE customer_id = $1::uuid ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: UpdateTransferRequestStatus :one
UPDATE transfer_requests
SET status = $2::transfer_request_status,
    posted_at = CASE WHEN $2::transfer_request_status = 'posted' THEN now() ELSE posted_at END
WHERE id = $1::uuid
RETURNING *;
