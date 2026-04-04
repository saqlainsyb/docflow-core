-- 000004_board_member_roles.up.sql
--
-- Adds board-level roles to board_members.
-- Before this migration, board_members tracked membership only (for private boards).
-- After this migration, board_members tracks roles for ALL boards:
--   owner  — full control, can delete board and transfer ownership
--   admin  — can manage members, rename board; cannot delete or remove owner
--   editor — can create columns and cards; cannot manage members
--
-- The creator of every board is back-filled as 'owner'.
-- Workspace-visibility boards that previously had no board_members rows
-- get their creator inserted now.

ALTER TABLE board_members
    ADD COLUMN role TEXT NOT NULL DEFAULT 'editor';

ALTER TABLE board_members
    ADD CONSTRAINT valid_board_role CHECK (role IN ('owner', 'admin', 'editor'));

-- Back-fill existing private board creators as owner.
-- These rows already exist in board_members (added at creation time).
UPDATE board_members bm
SET    role = 'owner'
FROM   boards b
WHERE  bm.board_id = b.id
  AND  bm.user_id  = b.created_by;

-- Ensure every board has exactly one owner row in board_members.
-- Workspace-visibility boards previously had NO board_members rows, so
-- we insert the creator. Private boards already have the row; ON CONFLICT
-- updates the role to 'owner' in case the back-fill above was a no-op
-- (e.g., created_by was NULL — those boards get skipped here intentionally).
INSERT INTO board_members (board_id, user_id, role)
SELECT b.id, b.created_by, 'owner'
FROM   boards b
WHERE  b.created_by IS NOT NULL
ON CONFLICT (board_id, user_id) DO UPDATE SET role = 'owner';