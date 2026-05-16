-- Reconciliation queries

-- name: CreateReconciliationBatch :one
INSERT INTO reconciliation_batches (
  source_system, source_file_name, source_reference, status, statement_date, expected_total_amount, expected_total_count, metadata
)
VALUES (@source_system, @source_file_name, @source_reference, @status::reconciliation_status, @statement_date::date, @expected_total_amount::numeric(20,8), @expected_total_count::int, @metadata::jsonb)
RETURNING *;

-- name: AddReconciliationItem :one
INSERT INTO reconciliation_items (
  reconciliation_batch_id, journal_transaction_id, journal_line_id, external_reference, currency_code, amount, metadata
)
VALUES (@reconciliation_batch_id::uuid, @journal_transaction_id::uuid, @journal_line_id::uuid, @external_reference, @currency_code::char(3), @amount::numeric(20,8), @metadata::jsonb)
RETURNING *;

-- name: ListReconciliationItemsForBatch :many
SELECT * FROM reconciliation_items WHERE reconciliation_batch_id = @reconciliation_batch_id::uuid ORDER BY created_at ASC;

-- name: MarkReconciliationItemMatched :one
UPDATE reconciliation_items SET status = 'matched', matched_at = now(), matched_amount = @matched_amount::numeric(20,8) WHERE id = @id::uuid AND status = 'pending' RETURNING *;
