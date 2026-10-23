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
