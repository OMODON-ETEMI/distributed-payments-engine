-- Locking helper queries

-- name: AcquireIdempotencyKeyForUpdate :one
SELECT id, idempotency_key, scope, request_hash, response_code, response_body, locked_at, expires_at, created_at, updated_at FROM idempotency_keys WHERE scope = @scope AND idempotency_key = @idempotency_key LIMIT 1 FOR UPDATE;

-- name: AcquireIdempotencyKeyForUpdateSkipLocked :one
SELECT id, idempotency_key, scope, request_hash, response_code, response_body, locked_at, expires_at, created_at, updated_at FROM idempotency_keys WHERE scope = @scope AND idempotency_key = @idempotency_key LIMIT 1 FOR UPDATE SKIP LOCKED;

-- Accounts: get row FOR UPDATE
-- name: GetAccountByIDForUpdate :one
SELECT id, customer_id, external_ref, account_number, account_type, status, currency_code, ledger_normal_side, metadata, opened_at, closed_at, created_at, updated_at, deleted_at FROM accounts WHERE id = @id::uuid LIMIT 1 FOR UPDATE;

-- name: GetAccountByIDForUpdateSkipLocked :one
SELECT id, customer_id, external_ref, account_number, account_type, status, currency_code, ledger_normal_side, metadata, opened_at, closed_at, created_at, updated_at, deleted_at FROM accounts WHERE id = @id::uuid LIMIT 1 FOR UPDATE SKIP LOCKED;

-- Balance projection: lock row for update
-- name: GetBalanceProjectionForUpdate :one
SELECT id, account_id, currency_code, balance_kind, ledger_balance, available_balance, held_balance, last_transaction_id, last_line_id, version, computed_at, created_at, updated_at FROM balance_projections WHERE account_id = @account_id::uuid AND currency_code = @currency_code::char(3) AND balance_kind = @balance_kind LIMIT 1 FOR UPDATE;

-- name: GetBalanceProjectionForUpdateSkipLocked :one
SELECT id, account_id, currency_code, balance_kind, ledger_balance, available_balance, held_balance, last_transaction_id, last_line_id, version, computed_at, created_at, updated_at FROM balance_projections WHERE account_id = @account_id::uuid AND currency_code = @currency_code::char(3) AND balance_kind = @balance_kind LIMIT 1 FOR UPDATE SKIP LOCKED;

-- Journal transaction: lock header for update
-- name: GetJournalTransactionByIDForUpdate :one
SELECT id, transaction_ref, transfer_request_id, idempotency_key_id, status, entry_type, accounting_date, effective_at, posted_at, reversed_transaction_id, reversal_of_transaction_id, source_system, source_event_id, description, metadata, created_at, updated_at, deleted_at FROM journal_transactions WHERE id = @id::uuid LIMIT 1 FOR UPDATE;

-- Transfer request: lock for update
-- name: GetTransferRequestByIDForUpdate :one
SELECT id, idempotency_key_id, customer_id, source_account_id, destination_account_id, currency_code, amount, fee_amount, status, client_reference, external_reference, failure_code, failure_reason, metadata, requested_at, reserved_at, submitted_at, posted_at, rejected_at, cancelled_at, expired_at, failed_at, created_at, updated_at, deleted_at FROM transfer_requests WHERE id = @id::uuid LIMIT 1 FOR UPDATE;

-- name: GetTransferRequestByIdempotencyKeyForUpdate :one
SELECT id, idempotency_key_id, customer_id, source_account_id, destination_account_id, currency_code, amount, fee_amount, status, client_reference, external_reference, failure_code, failure_reason, metadata, requested_at, reserved_at, submitted_at, posted_at, rejected_at, cancelled_at, expired_at, failed_at, created_at, updated_at, deleted_at FROM transfer_requests WHERE idempotency_key_id = @idempotency_key_id::uuid LIMIT 1 FOR UPDATE;
