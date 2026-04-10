BEGIN;

DELETE FROM accounts WHERE external_ref IN (
    'system_cash_ngn',
    'system_settlement_ngn',
    'system_fee_revenue_ngn',
    'system_suspense_ngn',
    'external_bank_ngn'
);

DELETE FROM customers WHERE external_ref = 'system_customer';

COMMIT;