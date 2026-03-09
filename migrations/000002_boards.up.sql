-- BOARDS
CREATE TABLE boards (
    id           UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    workspace_id UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title        TEXT        NOT NULL,
    visibility   TEXT        NOT NULL DEFAULT 'workspace',
    share_token  TEXT        UNIQUE,
    created_by   UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_visibility CHECK (visibility IN ('workspace', 'private'))
);

CREATE INDEX idx_boards_workspace   ON boards(workspace_id);
CREATE INDEX idx_boards_share_token ON boards(share_token) WHERE share_token IS NOT NULL;

-- BOARD MEMBERS
CREATE TABLE board_members (
    board_id  UUID        NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    user_id   UUID        NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    added_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (board_id, user_id)
);

CREATE INDEX idx_board_members_user ON board_members(user_id);

-- COLUMNS
CREATE TABLE columns (
    id         UUID             PRIMARY KEY DEFAULT uuid_generate_v4(),
    board_id   UUID             NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    title      TEXT             NOT NULL,
    position   DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_columns_board_position ON columns(board_id, position);

-- CARDS
CREATE TABLE cards (
    id          UUID             PRIMARY KEY DEFAULT uuid_generate_v4(),
    board_id    UUID             NOT NULL REFERENCES boards(id)   ON DELETE CASCADE,
    column_id   UUID             NOT NULL REFERENCES columns(id)  ON DELETE CASCADE,
    title       TEXT             NOT NULL,
    position    DOUBLE PRECISION NOT NULL,
    color       TEXT,
    assignee_id UUID             REFERENCES users(id) ON DELETE SET NULL,
    archived    BOOLEAN          NOT NULL DEFAULT FALSE,
    created_by  UUID             REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ      NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_color CHECK (
        color IS NULL OR
        color IN ('#EF4444','#F97316','#EAB308','#22C55E','#3B82F6','#A855F7')
    )
);

CREATE INDEX idx_cards_column_position ON cards(column_id, position) WHERE archived = FALSE;
CREATE INDEX idx_cards_board           ON cards(board_id)             WHERE archived = FALSE;
CREATE INDEX idx_cards_assignee        ON cards(assignee_id)          WHERE assignee_id IS NOT NULL;