CREATE INDEX IF NOT EXISTS idx_creazy_canvas_works_admin_kind_status_created
    ON creazy_canvas_works(kind, status, created_at DESC)
    WHERE deleted_at IS NULL;
