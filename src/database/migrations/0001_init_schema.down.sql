BEGIN;

DROP TRIGGER IF EXISTS trg_journal_lines_double_entry ON journal_lines;
DROP TRIGGER IF EXISTS trg_journal_lines_no_update ON journal_lines;
DROP TRIGGER IF EXISTS trg_journal_transactions_no_update ON journal_transactions;
DROP TRIGGER IF EXISTS trg_audit_logs_no_update ON audit_logs;
DROP TRIGGER IF EXISTS trg_reconciliation_items_no_update ON reconciliation_items;
DROP TRIGGER IF EXISTS trg_reconciliation_batches_no_delete ON reconciliation_batches;

DROP TRIGGER IF EXISTS trg_balance_projections_updated_at ON balance_projections;
DROP TRIGGER IF EXISTS trg_funds_holds_updated_at ON funds_holds;
DROP TRIGGER IF EXISTS trg_transfer_requests_updated_at ON transfer_requests;
DROP TRIGGER IF EXISTS trg_accounts_updated_at ON accounts;
DROP TRIGGER IF EXISTS trg_customers_updated_at ON customers;
DROP TRIGGER IF EXISTS trg_idempotency_keys_updated_at ON idempotency_keys;
DROP TRIGGER IF EXISTS trg_journal_transactions_updated_at ON journal_transactions;
DROP TRIGGER IF EXISTS trg_outbox_events_updated_at ON outbox_events;
DROP TRIGGER IF EXISTS trg_reconciliation_batches_updated_at ON reconciliation_batches;
DROP TRIGGER IF EXISTS trg_reconciliation_items_updated_at ON reconciliation_items;

-- ------------------------------------------------------------------
-- DROP FUNCTIONS
-- ------------------------------------------------------------------

DROP FUNCTION IF EXISTS validate_journal_transaction_double_entry();
DROP FUNCTION IF EXISTS prevent_updates_to_immutable_ledger_rows();
DROP FUNCTION IF EXISTS set_updated_at();

-- ------------------------------------------------------------------
-- DROP TABLES (reverse order)
-- ------------------------------------------------------------------

DROP TABLE IF EXISTS reconciliation_items;
DROP TABLE IF EXISTS reconciliation_batches;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS balance_projections;
DROP TABLE IF EXISTS funds_holds;
DROP TABLE IF EXISTS journal_lines;
DROP TABLE IF EXISTS journal_transactions;
DROP TABLE IF EXISTS transfer_requests;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS customers;

-- ------------------------------------------------------------------
-- DROP ENUMS
-- ------------------------------------------------------------------

DROP TYPE IF EXISTS reconciliation_item_status;
DROP TYPE IF EXISTS reconciliation_status;
DROP TYPE IF EXISTS audit_action;
DROP TYPE IF EXISTS outbox_status;
DROP TYPE IF EXISTS hold_status;
DROP TYPE IF EXISTS journal_line_side;
DROP TYPE IF EXISTS journal_transaction_status;
DROP TYPE IF EXISTS transfer_request_status;
DROP TYPE IF EXISTS account_status;
DROP TYPE IF EXISTS account_type;
DROP TYPE IF EXISTS customer_status;
DROP TYPE IF EXISTS ledger_balance_kind;

COMMIT;