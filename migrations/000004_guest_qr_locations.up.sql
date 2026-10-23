CREATE TABLE guest_qr_locations (
    event_id TEXT PRIMARY KEY,
    revision_id TEXT NOT NULL,
    qr_locations JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
