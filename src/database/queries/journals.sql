-- Journal transactions & lines

-- name: CreateJournalTransaction :one
INSERT INTO journal_transactions (
  transaction_ref, transfer_request_id, idempotency_key_id, status, entry_type, accounting_date, effective_at, source_system, source_event_id, description, metadata
)
VALUES ($1, $2::uuid, $3::uuid, $4::journal_transaction_status, $5, $6::date, $7::timestamptz, $8, $9, $10, $11::jsonb)
RETURNING *;

-- name: CreateJournalLine :one
INSERT INTO journal_lines (
  journal_transaction_id, line_number, account_id, side, amount, currency_code, balance_kind, memo, metadata
)
VALUES ($1::uuid, $2::int, $3::uuid, $4::journal_line_side, $5::numeric(20,8), $6::char(3), $7::ledger_balance_kind, $8, $9::jsonb)
RETURNING *;

-- name: GetJournalTransactionByRef :one
SELECT * FROM journal_transactions WHERE transaction_ref = $1 LIMIT 1;

-- name: ListJournalLinesForTransaction :many
SELECT * FROM journal_lines WHERE journal_transaction_id = $1::uuid ORDER BY line_number ASC;

-- name: MarkJournalTransactionPosted :one
UPDATE journal_transactions SET status = 'posted', posted_at = now() WHERE id = $1::uuid AND status <> 'posted' RETURNING *;
