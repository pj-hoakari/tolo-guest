CREATE TABLE guest_display_names (
    event_id TEXT PRIMARY KEY,
    revision_id TEXT NOT NULL,
    display_names JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
