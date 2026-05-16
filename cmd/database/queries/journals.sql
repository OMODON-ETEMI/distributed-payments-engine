-- Journal transactions & lines

-- name: CreateJournalTransaction :one
INSERT INTO journal_transactions (
  transaction_ref, transfer_request_id, idempotency_key_id, status, entry_type, accounting_date, effective_at, source_system, source_event_id, description, metadata
)
VALUES (@transaction_ref, @transfer_request_id::uuid, @idempotency_key_id::uuid, @status::journal_transaction_status, @entry_type, @accounting_date::date, @effective_at::timestamptz, @source_system, @source_event_id, @description, @metadata::jsonb)
RETURNING *;

-- name: CreateJournalLine :one
INSERT INTO journal_lines (
  journal_transaction_id, line_number, account_id, side, amount, currency_code, balance_kind, memo, metadata
)
VALUES (@journal_transaction_id::uuid, @line_number::int, @account_id::uuid, @side::journal_line_side, @amount::numeric(20,8), @currency_code::char(3), @balance_kind::ledger_balance_kind, @memo, @metadata::jsonb)
RETURNING *;

-- name: GetJournalTransactionByRef :one
SELECT * FROM journal_transactions WHERE transaction_ref = @transaction_ref LIMIT 1;

-- name: ListJournalLinesForTransaction :many
SELECT * FROM journal_lines WHERE journal_transaction_id = @journal_transaction_id::uuid ORDER BY line_number ASC;

-- name: MarkJournalTransactionPosted :one
UPDATE journal_transactions SET status = 'posted', posted_at = now() WHERE id = @id::uuid AND status <> 'posted' RETURNING *;
