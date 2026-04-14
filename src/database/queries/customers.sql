-- Customers queries

-- name: CreateCustomer :one
INSERT INTO customers (external_ref, full_name, email, phone, national_id, status, metadata)
VALUES ($1, $2, $3::citext, $4, $5, $6::customer_status, $7::jsonb)
RETURNING *;

-- name: GetCustomerByID :one
SELECT * FROM customers WHERE id = $1::uuid LIMIT 1;

-- name: GetCustomerByExternalRef :one
SELECT * FROM customers WHERE external_ref = $1 LIMIT 1;

-- name: ListCustomers :many
SELECT * FROM customers ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: UpdateCustomerStatus :one
UPDATE customers SET status = $2::customer_status WHERE id = $1::uuid RETURNING *;
