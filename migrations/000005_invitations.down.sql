-- migrations/000005_invitations.down.sql

DROP INDEX IF EXISTS idx_invitations_pending_email;
DROP INDEX IF EXISTS idx_invitations_workspace_status;
DROP INDEX IF EXISTS idx_invitations_token_hash;
DROP TABLE IF EXISTS workspace_invitations;