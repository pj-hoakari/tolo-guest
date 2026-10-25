CREATE TABLE guest_guide_maps (
    event_id TEXT PRIMARY KEY,
    revision_id TEXT NOT NULL,
    layout BYTEA NOT NULL,
    point_labels JSONB NOT NULL,
    group_labels JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
