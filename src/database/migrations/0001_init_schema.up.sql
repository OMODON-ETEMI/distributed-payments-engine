BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

-- -----------------------------------------------------------------------------
-- ENUMS
-- -----------------------------------------------------------------------------

DO $$ BEGIN
    CREATE TYPE customer_status AS ENUM ('active', 'suspended', 'closed');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE account_type AS ENUM ('system', 'settlement', 'fee', 'customer');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE account_status AS ENUM ('pending', 'active', 'frozen', 'closed');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE transfer_request_status AS ENUM (
        'pending',
        'reversed',
        'posted',
        'failed'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE journal_transaction_status AS ENUM ('pending', 'posted', 'reversed', 'voided', 'failed');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE journal_line_side AS ENUM ('debit', 'credit');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE hold_status AS ENUM ('active', 'released', 'consumed', 'expired');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE outbox_status AS ENUM ('pending', 'processing', 'published', 'failed', 'dead_letter');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE audit_action AS ENUM (
        'insert',
        'update',
        'delete',
        'status_change',
        'reconcile',
        'reverse',
        'void',
        'adjust'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE reconciliation_status AS ENUM ('pending', 'matched', 'mismatch', 'reviewed', 'resolved', 'closed');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE reconciliation_item_status AS ENUM ('pending', 'matched', 'exception');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE ledger_balance_kind AS ENUM ('ledger', 'available', 'held');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- -----------------------------------------------------------------------------
-- COMMON TIMESTAMP TRIGGER
-- -----------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

-- -----------------------------------------------------------------------------
-- CUSTOMERS
-- -----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS customers (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    external_ref        text NOT NULL,
    full_name           text NOT NULL,
    email               citext,
    phone               text,
    national_id         text,
    status              customer_status NOT NULL DEFAULT 'active',
    metadata            jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    deleted_at          timestamptz,
    CONSTRAINT customers_external_ref_uk UNIQUE (external_ref),
    CONSTRAINT customers_email_uk UNIQUE (email),
    CONSTRAINT customers_phone_uk UNIQUE (phone),
    CONSTRAINT customers_national_id_uk UNIQUE (national_id),
    CONSTRAINT customers_metadata_is_object CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX IF NOT EXISTS customers_status_idx ON customers (status);
CREATE INDEX IF NOT EXISTS customers_created_at_idx ON customers (created_at DESC);
CREATE INDEX IF NOT EXISTS customers_deleted_at_idx ON customers (deleted_at) WHERE deleted_at IS NOT NULL;

DROP TRIGGER IF EXISTS trg_customers_updated_at ON customers;
CREATE TRIGGER trg_customers_updated_at
BEFORE UPDATE ON customers
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- -----------------------------------------------------------------------------
-- ACCOUNTS
-- -----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS accounts (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id         uuid NOT NULL REFERENCES customers(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    external_ref        text NOT NULL,
    account_number      text NOT NULL,
    account_type        account_type NOT NULL,
    status              account_status NOT NULL DEFAULT 'pending',
    currency_code       char(3) NOT NULL,
    ledger_normal_side  journal_line_side NOT NULL,
    metadata            jsonb NOT NULL DEFAULT '{}'::jsonb,
    opened_at           timestamptz,
    closed_at           timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    deleted_at          timestamptz,
    CONSTRAINT accounts_external_ref_uk UNIQUE (external_ref),
    CONSTRAINT accounts_account_number_uk UNIQUE (account_number),
    CONSTRAINT accounts_currency_code_check CHECK (currency_code ~ '^[A-Z]{3}$'),
    CONSTRAINT accounts_metadata_is_object CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT accounts_closed_at_requires_closed CHECK (
        (closed_at IS NULL AND status <> 'closed') OR (closed_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS accounts_customer_id_idx ON accounts (customer_id);
CREATE INDEX IF NOT EXISTS accounts_status_idx ON accounts (status);
CREATE INDEX IF NOT EXISTS accounts_currency_status_idx ON accounts (currency_code, status);
CREATE INDEX IF NOT EXISTS accounts_deleted_at_idx ON accounts (deleted_at) WHERE deleted_at IS NOT NULL;

DROP TRIGGER IF EXISTS trg_accounts_updated_at ON accounts;
CREATE TRIGGER trg_accounts_updated_at
BEFORE UPDATE ON accounts
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- -----------------------------------------------------------------------------
-- IDEMPOTENCY KEYS
-- -----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS idempotency_keys (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key     text NOT NULL,
    scope               text NOT NULL,
    request_hash        bytea NOT NULL,
    response_code       integer,
    response_body       jsonb,
    locked_at           timestamptz,
    expires_at          timestamptz NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT idempotency_keys_scope_key_uk UNIQUE (scope, idempotency_key),
    CONSTRAINT idempotency_keys_scope_not_blank CHECK (length(btrim(scope)) > 0),
    CONSTRAINT idempotency_keys_key_not_blank CHECK (length(btrim(idempotency_key)) > 0),
    CONSTRAINT idempotency_keys_expires_future CHECK (expires_at > created_at)
);

CREATE INDEX IF NOT EXISTS idempotency_keys_expires_at_idx ON idempotency_keys (expires_at);
CREATE INDEX IF NOT EXISTS idempotency_keys_locked_at_idx ON idempotency_keys (locked_at);

DROP TRIGGER IF EXISTS trg_idempotency_keys_updated_at ON idempotency_keys;
CREATE TRIGGER trg_idempotency_keys_updated_at
BEFORE UPDATE ON idempotency_keys
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- -----------------------------------------------------------------------------
-- TRANSFER REQUESTS (business intent layer)
-- -----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS transfer_requests (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key_id      uuid NOT NULL REFERENCES idempotency_keys(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    customer_id             uuid NOT NULL REFERENCES customers(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    source_account_id       uuid NOT NULL REFERENCES accounts(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    destination_account_id  uuid NOT NULL REFERENCES accounts(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    currency_code           char(3) NOT NULL,
    amount                  numeric(20, 8) NOT NULL,
    fee_amount              numeric(20, 8) NOT NULL DEFAULT 0,
    status                  transfer_request_status NOT NULL DEFAULT 'pending',
    client_reference        text,
    external_reference      text,
    failure_code            text,
    failure_reason          text,
    metadata                jsonb NOT NULL DEFAULT '{}'::jsonb,
    requested_at            timestamptz NOT NULL DEFAULT now(),
    reserved_at             timestamptz,
    submitted_at            timestamptz,
    posted_at               timestamptz,
    rejected_at             timestamptz,
    cancelled_at            timestamptz,
    expired_at              timestamptz,
    failed_at               timestamptz,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    deleted_at              timestamptz,
    CONSTRAINT transfer_requests_idempotency_uk UNIQUE (idempotency_key_id),
    CONSTRAINT transfer_requests_client_reference_uk UNIQUE (client_reference),
    CONSTRAINT transfer_requests_external_reference_uk UNIQUE (external_reference),
    CONSTRAINT transfer_requests_amount_positive CHECK (amount > 0),
    CONSTRAINT transfer_requests_fee_nonnegative CHECK (fee_amount >= 0),
    CONSTRAINT transfer_requests_currency_code_check CHECK (currency_code ~ '^[A-Z]{3}$'),
    CONSTRAINT transfer_requests_metadata_is_object CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT transfer_requests_distinct_accounts CHECK (source_account_id <> destination_account_id)
);

CREATE INDEX IF NOT EXISTS transfer_requests_customer_id_idx ON transfer_requests (customer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS transfer_requests_source_account_idx ON transfer_requests (source_account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS transfer_requests_destination_account_idx ON transfer_requests (destination_account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS transfer_requests_status_idx ON transfer_requests (status, created_at DESC);
CREATE INDEX IF NOT EXISTS transfer_requests_external_reference_idx ON transfer_requests (external_reference) WHERE external_reference IS NOT NULL;
CREATE INDEX IF NOT EXISTS transfer_requests_deleted_at_idx ON transfer_requests (deleted_at) WHERE deleted_at IS NOT NULL;

DROP TRIGGER IF EXISTS trg_transfer_requests_updated_at ON transfer_requests;
CREATE TRIGGER trg_transfer_requests_updated_at
BEFORE UPDATE ON transfer_requests
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- -----------------------------------------------------------------------------
-- JOURNAL TRANSACTIONS (append-only source of truth header)
-- -----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS journal_transactions (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_ref         text NOT NULL,
    transfer_request_id     uuid REFERENCES transfer_requests(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    idempotency_key_id      uuid NOT NULL REFERENCES idempotency_keys(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    status                  journal_transaction_status NOT NULL DEFAULT 'pending',
    entry_type              text NOT NULL,
    accounting_date         date NOT NULL DEFAULT CURRENT_DATE,
    effective_at            timestamptz NOT NULL DEFAULT now(),
    posted_at               timestamptz,
    reversed_transaction_id uuid REFERENCES journal_transactions(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    reversal_of_transaction_id uuid REFERENCES journal_transactions(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    source_system           text NOT NULL DEFAULT 'core',
    source_event_id         text,
    description             text,
    metadata                jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    deleted_at              timestamptz,
    CONSTRAINT journal_transactions_ref_uk UNIQUE (transaction_ref),
    CONSTRAINT journal_transactions_idempotency_uk UNIQUE (idempotency_key_id),
    CONSTRAINT journal_transactions_source_event_uk UNIQUE (source_system, source_event_id),
    CONSTRAINT journal_transactions_metadata_is_object CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT journal_transactions_entry_type_not_blank CHECK (length(btrim(entry_type)) > 0),
    CONSTRAINT journal_transactions_source_system_not_blank CHECK (length(btrim(source_system)) > 0),
    CONSTRAINT journal_transactions_distinct_reversal CHECK (
        reversed_transaction_id IS NULL OR reversed_transaction_id <> id
    ),
    CONSTRAINT journal_transactions_distinct_reversal_of CHECK (
        reversal_of_transaction_id IS NULL OR reversal_of_transaction_id <> id
    )
);

CREATE INDEX IF NOT EXISTS journal_transactions_status_idx ON journal_transactions (status, created_at DESC);
CREATE INDEX IF NOT EXISTS journal_transactions_accounting_date_idx ON journal_transactions (accounting_date DESC, created_at DESC);
CREATE INDEX IF NOT EXISTS journal_transactions_transfer_request_idx ON journal_transactions (transfer_request_id);
CREATE INDEX IF NOT EXISTS journal_transactions_effective_at_idx ON journal_transactions (effective_at DESC);
CREATE INDEX IF NOT EXISTS journal_transactions_deleted_at_idx ON journal_transactions (deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS journal_transactions_source_event_idx ON journal_transactions (source_system, source_event_id) WHERE source_event_id IS NOT NULL;

DROP TRIGGER IF EXISTS trg_journal_transactions_updated_at ON journal_transactions;
CREATE TRIGGER trg_journal_transactions_updated_at
BEFORE UPDATE ON journal_transactions
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- -----------------------------------------------------------------------------
-- JOURNAL LINES (immutable double-entry lines; single source of truth)
-- -----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS journal_lines (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    journal_transaction_id uuid NOT NULL REFERENCES journal_transactions(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    line_number         integer NOT NULL,
    account_id          uuid NOT NULL REFERENCES accounts(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    side                journal_line_side NOT NULL,
    amount              numeric(20, 8) NOT NULL,
    currency_code       char(3) NOT NULL,
    balance_kind        ledger_balance_kind NOT NULL DEFAULT 'ledger',
    memo                text,
    metadata            jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT journal_lines_tx_line_uk UNIQUE (journal_transaction_id, line_number),
    CONSTRAINT journal_lines_amount_positive CHECK (amount > 0),
    CONSTRAINT journal_lines_currency_code_check CHECK (currency_code ~ '^[A-Z]{3}$'),
    CONSTRAINT journal_lines_metadata_is_object CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX IF NOT EXISTS journal_lines_transaction_id_idx ON journal_lines (journal_transaction_id, line_number);
CREATE INDEX IF NOT EXISTS journal_lines_account_id_idx ON journal_lines (account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS journal_lines_account_currency_idx ON journal_lines (account_id, currency_code, created_at DESC);
CREATE INDEX IF NOT EXISTS journal_lines_currency_idx ON journal_lines (currency_code, created_at DESC);
CREATE INDEX IF NOT EXISTS journal_lines_balance_kind_idx ON journal_lines (balance_kind, created_at DESC);

-- -----------------------------------------------------------------------------
-- FUNDS HOLDS (reservation layer; still ledger-backed)
-- -----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS funds_holds (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id          uuid NOT NULL REFERENCES accounts(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    transfer_request_id uuid REFERENCES transfer_requests(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    journal_transaction_id uuid REFERENCES journal_transactions(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    idempotency_key_id  uuid NOT NULL REFERENCES idempotency_keys(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    status              hold_status NOT NULL DEFAULT 'active',
    currency_code       char(3) NOT NULL,
    amount              numeric(20, 8) NOT NULL,
    remaining_amount    numeric(20, 8) NOT NULL,
    released_amount     numeric(20, 8) NOT NULL DEFAULT 0,
    captured_amount     numeric(20, 8) NOT NULL DEFAULT 0,
    reason_code         text,
    reason              text,
    expires_at          timestamptz,
    captured_at         timestamptz,
    released_at         timestamptz,
    cancelled_at        timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    deleted_at          timestamptz,
    CONSTRAINT funds_holds_idempotency_uk UNIQUE (idempotency_key_id),
    CONSTRAINT funds_holds_amount_positive CHECK (amount > 0),
    CONSTRAINT funds_holds_remaining_nonnegative CHECK (remaining_amount >= 0),
    CONSTRAINT funds_holds_released_nonnegative CHECK (released_amount >= 0),
    CONSTRAINT funds_holds_captured_nonnegative CHECK (captured_amount >= 0),
    CONSTRAINT funds_holds_components_sum CHECK (remaining_amount + released_amount + captured_amount = amount),
    CONSTRAINT funds_holds_currency_code_check CHECK (currency_code ~ '^[A-Z]{3}$')
);

CREATE INDEX IF NOT EXISTS funds_holds_account_status_idx ON funds_holds (account_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS funds_holds_transfer_request_idx ON funds_holds (transfer_request_id);
CREATE INDEX IF NOT EXISTS funds_holds_journal_transaction_idx ON funds_holds (journal_transaction_id);
CREATE INDEX IF NOT EXISTS funds_holds_expires_at_idx ON funds_holds (expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS funds_holds_deleted_at_idx ON funds_holds (deleted_at) WHERE deleted_at IS NOT NULL;

DROP TRIGGER IF EXISTS trg_funds_holds_updated_at ON funds_holds;
CREATE TRIGGER trg_funds_holds_updated_at
BEFORE UPDATE ON funds_holds
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- -----------------------------------------------------------------------------
-- BALANCE PROJECTIONS (derived, rebuildable, not source of truth)
-- -----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS balance_projections (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id          uuid NOT NULL REFERENCES accounts(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    currency_code       char(3) NOT NULL,
    balance_kind        ledger_balance_kind NOT NULL,
    ledger_balance      numeric(20, 8) NOT NULL DEFAULT 0,
    available_balance   numeric(20, 8) NOT NULL DEFAULT 0,
    held_balance        numeric(20, 8) NOT NULL DEFAULT 0,
    last_transaction_id uuid REFERENCES journal_transactions(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    last_line_id        uuid REFERENCES journal_lines(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    version             bigint NOT NULL DEFAULT 0,
    computed_at         timestamptz NOT NULL DEFAULT now(),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT balance_projections_account_currency_kind_uk UNIQUE (account_id, currency_code, balance_kind),
    CONSTRAINT balance_projections_currency_code_check CHECK (currency_code ~ '^[A-Z]{3}$'),
    CONSTRAINT balance_projections_balances_nonnegative CHECK (
        ledger_balance >= 0 AND available_balance >= 0 AND held_balance >= 0
    ),
    CONSTRAINT balance_projections_available_formula CHECK (available_balance + held_balance = ledger_balance)
);

CREATE INDEX IF NOT EXISTS balance_projections_account_idx ON balance_projections (account_id, currency_code, balance_kind);
CREATE INDEX IF NOT EXISTS balance_projections_currency_idx ON balance_projections (currency_code, balance_kind);
CREATE INDEX IF NOT EXISTS balance_projections_computed_at_idx ON balance_projections (computed_at DESC);

DROP TRIGGER IF EXISTS trg_balance_projections_updated_at ON balance_projections;
CREATE TRIGGER trg_balance_projections_updated_at
BEFORE UPDATE ON balance_projections
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- -----------------------------------------------------------------------------
-- OUTBOX EVENTS (reliable publication to Kafka/SNS/SQS/webhooks/etc.)
-- -----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS outbox_events (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type      text NOT NULL,
    aggregate_id        uuid NOT NULL,
    event_type          text NOT NULL,
    status              outbox_status NOT NULL DEFAULT 'pending',
    idempotency_key_id  uuid REFERENCES idempotency_keys(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    payload             jsonb NOT NULL,
    headers             jsonb NOT NULL DEFAULT '{}'::jsonb,
    partition_key       text,
    published_at       timestamptz,
    retry_count        integer NOT NULL DEFAULT 0,
    next_retry_at      timestamptz,
    error_code         text,
    error_message      text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT outbox_events_payload_is_object CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT outbox_events_headers_is_object CHECK (jsonb_typeof(headers) = 'object'),
    CONSTRAINT outbox_events_aggregate_not_blank CHECK (length(btrim(aggregate_type)) > 0),
    CONSTRAINT outbox_events_event_type_not_blank CHECK (length(btrim(event_type)) > 0),
    CONSTRAINT outbox_events_retry_count_nonnegative CHECK (retry_count >= 0)
);

CREATE INDEX IF NOT EXISTS outbox_events_status_idx ON outbox_events (status, created_at ASC);
CREATE INDEX IF NOT EXISTS outbox_events_next_retry_idx ON outbox_events (next_retry_at ASC) WHERE next_retry_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS outbox_events_aggregate_idx ON outbox_events (aggregate_type, aggregate_id, created_at DESC);
CREATE INDEX IF NOT EXISTS outbox_events_event_type_idx ON outbox_events (event_type, created_at DESC);
CREATE INDEX IF NOT EXISTS outbox_events_idempotency_idx ON outbox_events (idempotency_key_id) WHERE idempotency_key_id IS NOT NULL;

DROP TRIGGER IF EXISTS trg_outbox_events_updated_at ON outbox_events;
CREATE TRIGGER trg_outbox_events_updated_at
BEFORE UPDATE ON outbox_events
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- -----------------------------------------------------------------------------
-- AUDIT LOGS (append-only)
-- -----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS audit_logs (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_type          text NOT NULL,
    actor_id            uuid,
    action              audit_action NOT NULL,
    entity_type         text NOT NULL,
    entity_id           uuid,
    before_state        jsonb,
    after_state         jsonb,
    diff                jsonb,
    request_id          uuid,
    correlation_id      uuid,
    metadata            jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT audit_logs_metadata_is_object CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT audit_logs_before_state_is_object CHECK (before_state IS NULL OR jsonb_typeof(before_state) = 'object'),
    CONSTRAINT audit_logs_after_state_is_object CHECK (after_state IS NULL OR jsonb_typeof(after_state) = 'object'),
    CONSTRAINT audit_logs_diff_is_object CHECK (diff IS NULL OR jsonb_typeof(diff) = 'object'),
    CONSTRAINT audit_logs_actor_type_not_blank CHECK (length(btrim(actor_type)) > 0),
    CONSTRAINT audit_logs_entity_type_not_blank CHECK (length(btrim(entity_type)) > 0)
);

CREATE INDEX IF NOT EXISTS audit_logs_entity_idx ON audit_logs (entity_type, entity_id, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_actor_idx ON audit_logs (actor_type, actor_id, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_action_idx ON audit_logs (action, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_correlation_idx ON audit_logs (correlation_id, created_at DESC) WHERE correlation_id IS NOT NULL;

-- -----------------------------------------------------------------------------
-- RECONCILIATION TABLES
-- -----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS reconciliation_batches (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_system       text NOT NULL,
    source_file_name    text,
    source_reference    text,
    status              reconciliation_status NOT NULL DEFAULT 'pending',
    statement_date      date,
    expected_total_amount numeric(20, 8) NOT NULL DEFAULT 0,
    expected_total_count integer NOT NULL DEFAULT 0,
    matched_total_amount numeric(20, 8) NOT NULL DEFAULT 0,
    matched_total_count integer NOT NULL DEFAULT 0,
    mismatch_total_amount numeric(20, 8) NOT NULL DEFAULT 0,
    mismatch_total_count integer NOT NULL DEFAULT 0,
    started_at          timestamptz,
    finished_at         timestamptz,
    metadata            jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT reconciliation_batches_source_not_blank CHECK (length(btrim(source_system)) > 0),
    CONSTRAINT reconciliation_batches_metadata_is_object CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT reconciliation_batches_amounts_nonnegative CHECK (
        expected_total_amount >= 0 AND matched_total_amount >= 0 AND mismatch_total_amount >= 0
    ),
    CONSTRAINT reconciliation_batches_counts_nonnegative CHECK (
        expected_total_count >= 0 AND matched_total_count >= 0 AND mismatch_total_count >= 0
    )
);

CREATE INDEX IF NOT EXISTS reconciliation_batches_status_idx ON reconciliation_batches (status, created_at DESC);
CREATE INDEX IF NOT EXISTS reconciliation_batches_statement_date_idx ON reconciliation_batches (statement_date DESC);
CREATE INDEX IF NOT EXISTS reconciliation_batches_source_idx ON reconciliation_batches (source_system, created_at DESC);

DROP TRIGGER IF EXISTS trg_reconciliation_batches_updated_at ON reconciliation_batches;
CREATE TRIGGER trg_reconciliation_batches_updated_at
BEFORE UPDATE ON reconciliation_batches
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS reconciliation_items (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reconciliation_batch_id uuid NOT NULL REFERENCES reconciliation_batches(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    journal_transaction_id  uuid REFERENCES journal_transactions(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    journal_line_id         uuid REFERENCES journal_lines(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    external_reference      text,
    status                  reconciliation_item_status NOT NULL DEFAULT 'pending',
    currency_code           char(3) NOT NULL,
    amount                  numeric(20, 8) NOT NULL,
    matched_amount          numeric(20, 8) NOT NULL DEFAULT 0,
    variance_amount         numeric(20, 8) NOT NULL DEFAULT 0,
    variance_reason         text,
    matched_at              timestamptz,
    metadata                jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT reconciliation_items_currency_code_check CHECK (currency_code ~ '^[A-Z]{3}$'),
    CONSTRAINT reconciliation_items_amount_positive CHECK (amount > 0),
    CONSTRAINT reconciliation_items_matched_amount_nonnegative CHECK (matched_amount >= 0),
    CONSTRAINT reconciliation_items_variance_amount_nonnegative CHECK (variance_amount >= 0),
    CONSTRAINT reconciliation_items_metadata_is_object CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT reconciliation_items_external_reference_uk UNIQUE (reconciliation_batch_id, external_reference)
);

CREATE INDEX IF NOT EXISTS reconciliation_items_batch_status_idx ON reconciliation_items (reconciliation_batch_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS reconciliation_items_transaction_idx ON reconciliation_items (journal_transaction_id);
CREATE INDEX IF NOT EXISTS reconciliation_items_line_idx ON reconciliation_items (journal_line_id);
CREATE INDEX IF NOT EXISTS reconciliation_items_external_reference_idx ON reconciliation_items (external_reference) WHERE external_reference IS NOT NULL;

DROP TRIGGER IF EXISTS trg_reconciliation_items_updated_at ON reconciliation_items;
CREATE TRIGGER trg_reconciliation_items_updated_at
BEFORE UPDATE ON reconciliation_items
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- -----------------------------------------------------------------------------
-- LEDGER INVARIANTS HELPERS
-- -----------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION validate_journal_transaction_double_entry()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    tx_id      uuid;
    debit_sum  numeric(20, 8);
    credit_sum numeric(20, 8);
    line_count integer;
BEGIN
    tx_id := COALESCE(NEW.journal_transaction_id, OLD.journal_transaction_id);

    SELECT
        COUNT(*)::int,
        COALESCE(SUM(CASE WHEN side = 'debit' THEN amount ELSE 0 END), 0),
        COALESCE(SUM(CASE WHEN side = 'credit' THEN amount ELSE 0 END), 0)
    INTO line_count, debit_sum, credit_sum
    FROM journal_lines
    WHERE journal_transaction_id = tx_id;

    IF line_count < 2 THEN
        RAISE EXCEPTION 'journal_transaction % must contain at least two lines', tx_id;
    END IF;

    IF debit_sum <> credit_sum THEN
        RAISE EXCEPTION 'journal_transaction % is unbalanced: debit_sum=% credit_sum=%', tx_id, debit_sum, credit_sum;
    END IF;

    RETURN COALESCE(NEW, OLD);
END;
$$;

CREATE OR REPLACE FUNCTION prevent_updates_to_immutable_ledger_rows()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% is immutable and cannot be updated or deleted', TG_TABLE_NAME;
END;
$$;

-- Enforce append-only on the source-of-truth tables.
DROP TRIGGER IF EXISTS trg_journal_transactions_no_update ON journal_transactions;
CREATE TRIGGER trg_journal_transactions_no_update
BEFORE UPDATE OR DELETE ON journal_transactions
FOR EACH ROW EXECUTE FUNCTION prevent_updates_to_immutable_ledger_rows();

DROP TRIGGER IF EXISTS trg_journal_lines_no_update ON journal_lines;
CREATE TRIGGER trg_journal_lines_no_update
BEFORE UPDATE OR DELETE ON journal_lines
FOR EACH ROW EXECUTE FUNCTION prevent_updates_to_immutable_ledger_rows();

DROP TRIGGER IF EXISTS trg_journal_lines_double_entry ON journal_lines;
CREATE CONSTRAINT TRIGGER trg_journal_lines_double_entry
AFTER INSERT ON journal_lines
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_journal_transaction_double_entry();

DROP TRIGGER IF EXISTS trg_audit_logs_no_update ON audit_logs;
CREATE TRIGGER trg_audit_logs_no_update
BEFORE UPDATE OR DELETE ON audit_logs
FOR EACH ROW EXECUTE FUNCTION prevent_updates_to_immutable_ledger_rows();

DROP TRIGGER IF EXISTS trg_reconciliation_items_no_update ON reconciliation_items;
CREATE TRIGGER trg_reconciliation_items_no_update
BEFORE UPDATE OR DELETE ON reconciliation_items
FOR EACH ROW EXECUTE FUNCTION prevent_updates_to_immutable_ledger_rows();

DROP TRIGGER IF EXISTS trg_reconciliation_batches_no_delete ON reconciliation_batches;
CREATE TRIGGER trg_reconciliation_batches_no_delete
BEFORE DELETE ON reconciliation_batches
FOR EACH ROW EXECUTE FUNCTION prevent_updates_to_immutable_ledger_rows();

-- Balance projections can be rebuilt, but still only updated explicitly by services.
-- No auto trigger here; sqlc/pgx services should use version checks in application logic.

-- -----------------------------------------------------------------------------
-- COMMENTS FOR MAINTAINERSHIP
-- -----------------------------------------------------------------------------

COMMENT ON TABLE journal_transactions IS 'Append-only journal transaction header. Source of truth for all monetary state changes.';
COMMENT ON TABLE journal_lines IS 'Append-only double-entry lines. Balances must be derived from this table and journal_transactions.';
COMMENT ON TABLE balance_projections IS 'Materialized derived balance state; rebuildable from the ledger at any time.';
COMMENT ON TABLE outbox_events IS 'Transactional outbox for reliable event publication to external systems.';
COMMENT ON TABLE audit_logs IS 'Append-only audit trail for operational and compliance review.';
COMMENT ON TABLE reconciliation_batches IS 'External statement or rail reconciliation batch header.';
COMMENT ON TABLE reconciliation_items IS 'Reconciliation line items mapped to journal entries or external records.';

COMMIT;