-- name: CreateIdempotencyKey :one
INSERT INTO idempotency_keys (
  idempotency_key, scope, request_hash, response_code, response_body, locked_at, expires_at
)
VALUES (@idempotency_key, @scope, @request_hash::bytea, @response_code, @response_body::jsonb, @locked_at::timestamptz, @expires_at::timestamptz)
RETURNING id, idempotency_key, scope, request_hash, response_code, response_body, locked_at, expires_at, created_at, updated_at;

-- name: GetIdempotencyKeyByScopeAndKey :one
SELECT id, idempotency_key, scope, request_hash, response_code, response_body, locked_at, expires_at, created_at, updated_at FROM idempotency_keys WHERE scope = @scope AND idempotency_key = @idempotency_key LIMIT 1;

-- name: GetIdempotencyKeyByID :one
SELECT id, idempotency_key, scope, request_hash, response_code, response_body, locked_at, expires_at, created_at, updated_at FROM idempotency_keys WHERE id = @id::uuid LIMIT 1;

-- name: UpdateIdempotencyKeyResponse :one
UPDATE idempotency_keys SET response_code = @response_code, response_body = @response_body::jsonb WHERE id = @id::uuid RETURNING id, idempotency_key, scope, request_hash, response_code, response_body, locked_at, expires_at, created_at, updated_at;
