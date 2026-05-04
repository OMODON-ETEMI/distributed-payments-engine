-- Customers queries

-- name: CreateCustomer :one
INSERT INTO customers (external_ref, full_name, email, phone, national_id, status, metadata)
VALUES (@external_ref, @full_name, @email::citext, @phone, @national_id, @status::customer_status, @metadata::jsonb)
RETURNING *;

-- name: GetCustomerByID :one
SELECT * FROM customers WHERE id = @id::uuid LIMIT 1;

-- name: GetCustomerByExternalRef :one
SELECT * FROM customers WHERE external_ref = @external_ref LIMIT 1;

-- name: ListCustomers :many
SELECT * FROM customers ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: UpdateCustomerStatus :one
UPDATE customers SET status = @status::customer_status WHERE id = @id::uuid RETURNING *;
