CREATE TABLE guest_qr_accesses (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT NOT NULL,
    qr_location_id TEXT NOT NULL,
    tenant_public_id TEXT NOT NULL,
    opened_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reported_at TIMESTAMPTZ
);
