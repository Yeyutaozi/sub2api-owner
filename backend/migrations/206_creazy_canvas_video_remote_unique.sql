-- Keep one visible work for a gateway task. Historical duplicate metadata is
-- soft-deleted before the partial unique index is installed.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY user_id, api_key_id, gateway_type, gateway_remote_id
               ORDER BY
                   CASE
                       WHEN status = 'succeeded' THEN 0
                       WHEN status IN ('created', 'queued', 'running') THEN 1
                       ELSE 2
                   END,
                   updated_at DESC,
                   id DESC
           ) AS duplicate_rank
    FROM creazy_canvas_works
    WHERE deleted_at IS NULL
      AND kind = 'video'
      AND gateway_remote_id <> ''
)
UPDATE creazy_canvas_works AS works
SET deleted_at = NOW(), updated_at = NOW()
FROM ranked
WHERE works.id = ranked.id
  AND ranked.duplicate_rank > 1;

CREATE UNIQUE INDEX IF NOT EXISTS uq_creazy_canvas_works_gateway_remote_owner
    ON creazy_canvas_works(user_id, api_key_id, gateway_type, gateway_remote_id)
    WHERE deleted_at IS NULL AND kind = 'video' AND gateway_remote_id <> '';
