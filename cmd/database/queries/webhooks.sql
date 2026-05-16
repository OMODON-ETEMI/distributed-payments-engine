-- name: CreateIncomingWebhook :one
INSERT INTO incoming_webhooks (
    provider,
    external_event_id,
    event_type,
    payload,
    headers,
    status
) VALUES (
    @provider,
    @external_event_id,
    @event_type,
    @payload::jsonb,
    @headers::jsonb,
    'pending'
) RETURNING *;

-- name: GetIncomingWebhookByID :one
SELECT * FROM incoming_webhooks WHERE id = @id::uuid LIMIT 1;

-- name: GetIncomingWebhookByExternalID :one
-- Used to prevent duplicate processing of the same event from the same provider
SELECT * FROM incoming_webhooks 
WHERE provider = @provider AND external_event_id = @external_event_id 
LIMIT 1;

-- name: ListPendingIncomingWebhooks :many
SELECT * FROM incoming_webhooks
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: UpdateIncomingWebhookStatus :one
UPDATE incoming_webhooks
SET status = @status,
    processed_at = CASE WHEN @status = 'processed' THEN now() ELSE processed_at END,
    error_message = @error_message,
    updated_at = now()
WHERE id = @id::uuid
RETURNING *;