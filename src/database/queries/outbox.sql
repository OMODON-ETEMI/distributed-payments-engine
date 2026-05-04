-- name: CreateOutboxEvent :one
INSERT INTO outbox_events (
    aggregate_type,
    aggregate_id,
    event_type,
    status,
    idempotency_key_id,
    payload,
    headers,
    partition_key
)
VALUES (
    @aggregate_type,
    @aggregate_id::uuid,
    @event_type,
    'pending',
    @idempotency_key_id::uuid,
    @payload::jsonb,
    @headers::jsonb,
    @partition_key
)
RETURNING *;

-- name: ListPendingOutboxEvents :many
SELECT *
FROM outbox_events
WHERE status IN ('pending', 'failed')
  AND (next_retry_at IS NULL OR next_retry_at <= now())
ORDER BY created_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxEventPublished :one
UPDATE outbox_events
SET status      = 'published',
    published_at = now()
WHERE id = @id::uuid
RETURNING *;

-- name: MarkOutboxEventFailed :one
UPDATE outbox_events
SET status        = 'failed',
    retry_count   = retry_count + 1,
    next_retry_at = now() + (power(2, retry_count) * interval '1 second'),
    error_code    = @error_code,
    error_message = @error_message
WHERE id = @id::uuid
RETURNING *;

-- name: MarkOutboxEventDeadLetter :one
UPDATE outbox_events
SET status = 'dead_letter'
WHERE id = @id::uuid
RETURNING *;