-- Accounts queries

-- name: CreateAccount :one
INSERT INTO accounts (
  customer_id, external_ref, account_number, account_type, status, currency_code, ledger_normal_side, metadata, opened_at
)
VALUES ($1::uuid, $2, $3, $4::account_type, $5::account_status, $6::char(3), $7::journal_line_side, $8::jsonb, $9::timestamptz)
RETURNING *;

-- name: GetAccountByID :one
SELECT * FROM accounts WHERE id = $1::uuid LIMIT 1;

-- name: GetAccountByExternalRef :one
SELECT * FROM accounts WHERE external_ref = $1 LIMIT 1;

-- name: GetAccountByNumber :one
SELECT * FROM accounts WHERE account_number = $1 LIMIT 1;

-- name: ListAccountsByCustomer :many
SELECT * FROM accounts WHERE customer_id = $1::uuid ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: UpdateAccountStatus :one
UPDATE accounts SET status = $2::account_status, closed_at = $3::timestamptz WHERE id = $1::uuid RETURNING *;
