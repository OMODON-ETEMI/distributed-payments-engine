BEGIN;

-- ------------------------------------------------------------------
-- SYSTEM CUSTOMER
-- ------------------------------------------------------------------

INSERT INTO customers (
    id,
    external_ref,
    full_name,
    email,
    status,
    metadata
)
SELECT
    gen_random_uuid(),
    'system_customer',
    'System Ledger Customer',
    'system@corebank.local',
    'active',
    '{}'::jsonb
WHERE NOT EXISTS (
    SELECT 1 FROM customers WHERE external_ref = 'system_customer'
);

-- ------------------------------------------------------------------
-- ------------------------------------------------------------------

-- CASH ACCOUNT
INSERT INTO accounts (
    customer_id,
    external_ref,
    account_number,
    account_type,
    status,
    currency_code,
    ledger_normal_side,
    metadata,
    opened_at
)
SELECT
    c.id,
    'system_cash_ngn',
    '0000000001',
    'asset',
    'active',
    'NGN',
    'debit',
    '{}'::jsonb,
    now()
FROM customers c
WHERE c.external_ref = 'system_customer'
AND NOT EXISTS (
    SELECT 1 FROM accounts WHERE external_ref = 'system_cash_ngn'
);

-- SETTLEMENT ACCOUNT
INSERT INTO accounts (
    customer_id,
    external_ref,
    account_number,
    account_type,
    status,
    currency_code,
    ledger_normal_side,
    metadata,
    opened_at
)
SELECT
    c.id,
    'system_settlement_ngn',
    '0000000002',
    'liability',
    'active',
    'NGN',
    'credit',
    '{}'::jsonb,
    now()
FROM customers c
WHERE c.external_ref = 'system_customer'
AND NOT EXISTS (
    SELECT 1 FROM accounts WHERE external_ref = 'system_settlement_ngn'
);

-- FEE REVENUE ACCOUNT
INSERT INTO accounts (
    customer_id,
    external_ref,
    account_number,
    account_type,
    status,
    currency_code,
    ledger_normal_side,
    metadata,
    opened_at
)
SELECT
    c.id,
    'system_fee_revenue_ngn',
    '0000000003',
    'revenue',
    'active',
    'NGN',
    'credit',
    '{}'::jsonb,
    now()
FROM customers c
WHERE c.external_ref = 'system_customer'
AND NOT EXISTS (
    SELECT 1 FROM accounts WHERE external_ref = 'system_fee_revenue_ngn'
);

-- SUSPENSE ACCOUNT
INSERT INTO accounts (
    customer_id,
    external_ref,
    account_number,
    account_type,
    status,
    currency_code,
    ledger_normal_side,
    metadata,
    opened_at
)
SELECT
    c.id,
    'system_suspense_ngn',
    '0000000004',
    'liability',
    'active',
    'NGN',
    'credit',
    '{}'::jsonb,
    now()
FROM customers c
WHERE c.external_ref = 'system_customer'
AND NOT EXISTS (
    SELECT 1 FROM accounts WHERE external_ref = 'system_suspense_ngn'
);

-- EXTERNAL BANK ACCOUNT
INSERT INTO accounts (
    customer_id,
    external_ref,
    account_number,
    account_type,
    status,
    currency_code,
    ledger_normal_side,
    metadata,
    opened_at
)
SELECT
    c.id,
    'external_bank_ngn',
    '0000000005',
    'asset',
    'active',
    'NGN',
    'debit',
    '{}'::jsonb,
    now()
FROM customers c
WHERE c.external_ref = 'system_customer'
AND NOT EXISTS (
    SELECT 1 FROM accounts WHERE external_ref = 'external_bank_ngn'
);

COMMIT;