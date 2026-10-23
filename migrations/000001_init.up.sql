CREATE TABLE guest_settings (
    event_id TEXT PRIMARY KEY,
    tenant_public_id TEXT NOT NULL,
    page_compositions JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
