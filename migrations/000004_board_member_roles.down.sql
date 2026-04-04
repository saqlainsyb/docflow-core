-- 000004_board_member_roles.down.sql
ALTER TABLE board_members DROP CONSTRAINT IF EXISTS valid_board_role;
ALTER TABLE board_members DROP COLUMN IF EXISTS role;