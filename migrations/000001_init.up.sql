CREATE TABLE guest_settings (
    event_id TEXT PRIMARY KEY,
    tenant_public_id TEXT NOT NULL,
    page_compositions JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE guest_statuses (
    event_id TEXT PRIMARY KEY,
    sequence BIGINT NOT NULL,
    taken_at TIMESTAMPTZ NOT NULL,
    crowd JSONB NOT NULL,
    queues JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE guest_messages (
    event_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    body TEXT NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (event_id, message_id)
);

CREATE TABLE guest_display_names (
    event_id TEXT PRIMARY KEY,
    revision_id TEXT NOT NULL,
    display_names JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE guest_qr_locations (
    event_id TEXT PRIMARY KEY,
    revision_id TEXT NOT NULL,
    qr_locations JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE guest_qr_accesses (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT NOT NULL,
    qr_location_id TEXT NOT NULL,
    tenant_public_id TEXT NOT NULL,
    opened_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
