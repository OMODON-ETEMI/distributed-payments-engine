-- name: CreateAuditLog :one
INSERT INTO audit_logs (
    actor_type,
    actor_id,
    action,
    entity_type,
    entity_id,
    before_state,
    after_state,
    diff,
    request_id,
    correlation_id,
    metadata
)
VALUES (
    @actor_type,
    @actor_id::uuid,
    @action::audit_action,
    @entity_type,
    @entity_id::uuid,
    @before_state::jsonb,
    @after_state::jsonb,
    @diff::jsonb,
    @request_id::uuid,
    @correlation_id::uuid,
    @metadata::jsonb
)
RETURNING *;