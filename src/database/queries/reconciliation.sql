-- Reconciliation queries

-- name: CreateReconciliationBatch :one
INSERT INTO reconciliation_batches (
  source_system, source_file_name, source_reference, status, statement_date, expected_total_amount, expected_total_count, metadata
)
VALUES ($1, $2, $3, $4::reconciliation_status, $5::date, $6::numeric(20,8), $7::int, $8::jsonb)
RETURNING *;

-- name: AddReconciliationItem :one
INSERT INTO reconciliation_items (
  reconciliation_batch_id, journal_transaction_id, journal_line_id, external_reference, currency_code, amount, metadata
)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5::char(3), $6::numeric(20,8), $7::jsonb)
RETURNING *;

-- name: ListReconciliationItemsForBatch :many
SELECT * FROM reconciliation_items WHERE reconciliation_batch_id = $1::uuid ORDER BY created_at ASC;

-- name: MarkReconciliationItemMatched :one
UPDATE reconciliation_items SET status = 'matched', matched_at = now(), matched_amount = $2::numeric(20,8) WHERE id = $1::uuid AND status = 'pending' RETURNING *;
