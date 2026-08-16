-- Persist user-owned node workflow documents separately from generated works.

CREATE TABLE IF NOT EXISTS creazy_canvas_documents (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        VARCHAR(120) NOT NULL DEFAULT 'Untitled workflow',
    graph_json  JSONB NOT NULL DEFAULT '{"nodes":[],"edges":[],"viewport":{"x":0,"y":0,"zoom":1}}',
    revision    BIGINT NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    CONSTRAINT creazy_canvas_documents_graph_object_check
        CHECK (jsonb_typeof(graph_json) = 'object'),
    CONSTRAINT creazy_canvas_documents_revision_check
        CHECK (revision > 0)
);

CREATE INDEX IF NOT EXISTS idx_creazy_canvas_documents_user_updated
    ON creazy_canvas_documents(user_id, updated_at DESC, id DESC)
    WHERE deleted_at IS NULL;

COMMENT ON TABLE creazy_canvas_documents IS
    'User-owned Creazy Canvas node graphs; media and generation jobs remain in their dedicated stores.';
