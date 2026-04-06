-- migrations/000005_invitations.up.sql

CREATE TABLE workspace_invitations (
    id            UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    workspace_id  UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    invited_email TEXT        NOT NULL,
    token_hash    TEXT        NOT NULL,
    invited_by    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role          TEXT        NOT NULL DEFAULT 'member',
    status        TEXT        NOT NULL DEFAULT 'pending',
    expires_at    TIMESTAMPTZ NOT NULL,
    accepted_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_invite_role CHECK (role IN ('admin', 'member')),
    CONSTRAINT valid_status      CHECK (status IN ('pending', 'accepted', 'cancelled'))
);

-- Fast O(1) token lookup — called on every accept click, must be instant.
CREATE UNIQUE INDEX idx_invitations_token_hash
    ON workspace_invitations(token_hash);

-- List all pending invitations for a workspace management page.
CREATE INDEX idx_invitations_workspace_status
    ON workspace_invitations(workspace_id, status);

-- Prevent duplicate pending invitations for the same email in the same workspace.
-- Partial index: only applies when status = 'pending', so a previously
-- cancelled/accepted invite does not block a fresh one being sent.
CREATE UNIQUE INDEX idx_invitations_pending_email
    ON workspace_invitations(workspace_id, invited_email)
    WHERE status = 'pending';