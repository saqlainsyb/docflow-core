-- DOCUMENTS
CREATE TABLE documents (
    id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    card_id         UUID        NOT NULL UNIQUE REFERENCES cards(id) ON DELETE CASCADE,
    snapshot        BYTEA,
    snapshot_clock  INTEGER     NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- DOCUMENT UPDATES
CREATE TABLE document_updates (
    id          BIGSERIAL   PRIMARY KEY,
    document_id UUID        NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    update_data BYTEA       NOT NULL,
    clock       INTEGER     NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_document_updates_sync
    ON document_updates(document_id, clock);

CREATE UNIQUE INDEX idx_document_updates_unique_clock
    ON document_updates(document_id, clock);