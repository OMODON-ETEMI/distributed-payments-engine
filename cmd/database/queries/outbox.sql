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
WITH selected_events AS (
    SELECT id 
    FROM outbox_events
    WHERE status IN ('pending', 'failed')
      AND (next_retry_at IS NULL OR next_retry_at <= now())
    ORDER BY created_at ASC
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
UPDATE outbox_events
SET 
    status = 'processing',
    updated_at = now()
FROM selected_events
WHERE outbox_events.id = selected_events.id
RETURNING outbox_events.*;

-- name: ResetStuckOutboxEvent :exec 
UPDATE outbox_events
SET status = 'pending',
    retry_count = retry_count + 1,
    updated_at = now()
WHERE status = 'processing' AND updated_at < now() - INTERVAL '5 minutes';


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