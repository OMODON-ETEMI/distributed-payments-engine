-- Accounts queries

-- name: CreateAccount :one
INSERT INTO accounts (
  customer_id, external_ref, account_number, account_type, status, currency_code, ledger_normal_side, metadata, opened_at
)
VALUES (@customer_id::uuid, @external_ref, @account_number, @account_type::account_type, @status::account_status, @currency_code::char(3), @ledger_normal_side::journal_line_side, @metadata::jsonb, @opened_at::timestamptz)
RETURNING *;

-- name: GetAccountByID :one
SELECT * FROM accounts WHERE id = @id::uuid LIMIT 1;

-- name: GetAccountByExternalRef :one
SELECT * FROM accounts WHERE external_ref = @external_ref LIMIT 1;

-- name: GetAccountByNumber :one
SELECT * FROM accounts WHERE account_number = @account_number LIMIT 1;

-- name: ListAccountsByCustomer :many
SELECT * FROM accounts WHERE customer_id = @customer_id::uuid ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: UpdateAccountStatus :one
UPDATE accounts SET status = @status::account_status, closed_at = @closed_at::timestamptz WHERE id = @id::uuid RETURNING *;
