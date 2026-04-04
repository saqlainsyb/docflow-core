package services

import (
	"context"
	"errors"

	"github.com/saqlainsyb/docflow-core/internal/config"
	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

type BoardService struct {
	boardRepo     *repositories.BoardRepository
	workspaceRepo *repositories.WorkspaceRepository
	cfg           *config.Config
}

func NewBoardService(
	boardRepo *repositories.BoardRepository,
	workspaceRepo *repositories.WorkspaceRepository,
	cfg *config.Config,
) *BoardService {
	return &BoardService{
		boardRepo:     boardRepo,
		workspaceRepo: workspaceRepo,
		cfg:           cfg,
	}
}

// ── Role helpers ──────────────────────────────────────────────────────────────

// ResolveBoardRole returns the effective board-level role for a user.
//
// Resolution order:
//  1. If the user has an explicit 'owner' role in board_members → "owner"
//  2. Workspace owner/admin → at least "admin" (unless they hold 'owner' above)
//  3. Explicit board role from board_members → that role
//  4. Workspace member with no board_members row → "editor" (default for
//     workspace-visibility boards; private boards would have been blocked earlier)
func (s *BoardService) ResolveBoardRole(ctx context.Context, boardID, userID, workspaceRole string) (string, error) {
	boardRole, err := s.boardRepo.GetBoardMemberRole(ctx, boardID, userID)
	if err != nil && !errors.Is(err, repositories.ErrNotFound) {
		return "", err
	}

	// Explicit board owner always wins — even workspace admins cannot override this.
	if boardRole == "owner" {
		return "owner", nil
	}

	// Workspace owner/admin get board-admin level minimum.
	if workspaceRole == "owner" || workspaceRole == "admin" {
		return "admin", nil
	}

	// Regular workspace member with an explicit board role.
	if boardRole != "" {
		return boardRole, nil
	}

	// Regular workspace member with no board_members row.
	// For workspace-visibility boards this is fine — they get editor access.
	// For private boards, checkAccess() will have already rejected them.
	return "editor", nil
}

// requireBoardRole returns ErrInsufficientPermissions unless the resolved
// board role is one of the allowed values.
func requireBoardRole(boardRole string, allowed ...string) error {
	for _, a := range allowed {
		if boardRole == a {
			return nil
		}
	}
	return ErrInsufficientPermissions
}

// ── Board CRUD ────────────────────────────────────────────────────────────────

// ListBoards returns all boards in a workspace visible to the requesting user.
func (s *BoardService) ListBoards(ctx context.Context, workspaceID, userID string) ([]models.BoardResponse, error) {
	return s.boardRepo.FindByWorkspace(ctx, workspaceID, userID)
}

// CreateBoard creates a new board. Any workspace member can create a board.
// The creator is always inserted as 'owner' in board_members by the repository.
func (s *BoardService) CreateBoard(ctx context.Context, workspaceID, userID string, req models.CreateBoardRequest) (*models.Board, error) {
	visibility := "workspace"
	if req.Visibility != nil {
		visibility = *req.Visibility
	}

	return s.boardRepo.Create(ctx, workspaceID, req.Title, visibility, userID)
}

// GetBoardDetail returns the full board with columns, cards, and members.
// myBoardRole is the caller's already-resolved board role (from middleware).
func (s *BoardService) GetBoardDetail(ctx context.Context, boardID, userID, workspaceRole, boardRole string) (*models.BoardDetailResponse, error) {
	if err := s.checkAccess(ctx, boardID, userID, workspaceRole); err != nil {
		return nil, err
	}

	detail, err := s.boardRepo.GetBoardDetail(ctx, boardID)
	if err != nil {
		return nil, err
	}

	detail.MyBoardRole = boardRole
	return detail, nil
}

// UpdateBoard changes a board's title and/or visibility.
//   - Renaming: board owner or admin
//   - Changing visibility: board owner only (visibility changes affect who can see the board)
func (s *BoardService) UpdateBoard(ctx context.Context, boardID, userID, workspaceRole, boardRole string, req models.UpdateBoardRequest) (*models.Board, error) {
	if err := s.checkAccess(ctx, boardID, userID, workspaceRole); err != nil {
		return nil, err
	}

	if req.Visibility != nil {
		if err := requireBoardRole(boardRole, "owner"); err != nil {
			return nil, err
		}
	}

	if req.Title != nil {
		if err := requireBoardRole(boardRole, "owner", "admin"); err != nil {
			return nil, err
		}
	}

	board, err := s.boardRepo.Update(ctx, boardID, req.Title, req.Visibility)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return board, nil
}

// DeleteBoard permanently removes a board.
// Allowed for: board owner OR workspace owner (workspace owners are responsible
// for their workspace and can clean up any board).
func (s *BoardService) DeleteBoard(ctx context.Context, boardID, userID, workspaceRole, boardRole string) error {
	if err := s.checkAccess(ctx, boardID, userID, workspaceRole); err != nil {
		return err
	}

	if boardRole != "owner" && workspaceRole != "owner" {
		return ErrInsufficientPermissions
	}

	if err := s.boardRepo.Delete(ctx, boardID); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	return nil
}

// ── Board membership ──────────────────────────────────────────────────────────

// ListBoardMembers returns all explicit board members with their board roles.
func (s *BoardService) ListBoardMembers(ctx context.Context, boardID, userID, workspaceRole string) ([]models.BoardMember, error) {
	if err := s.checkAccess(ctx, boardID, userID, workspaceRole); err != nil {
		return nil, err
	}

	members, err := s.boardRepo.ListBoardMembers(ctx, boardID)
	if err != nil {
		return nil, err
	}

	return members, nil
}

// AddBoardMember adds a user to a board with the specified role (default: editor).
// Requires board owner or admin.
// The target user must be a workspace member — you can't add someone to a board
// who isn't already in the workspace.
func (s *BoardService) AddBoardMember(ctx context.Context, boardID, requesterID, workspaceRole, boardRole string, req models.AddBoardMemberRequest) error {
	if err := s.checkAccess(ctx, boardID, requesterID, workspaceRole); err != nil {
		return err
	}

	if err := requireBoardRole(boardRole, "owner", "admin"); err != nil {
		return err
	}

	// Admins can only add editors — only the owner can grant admin.
	targetRole := "editor"
	if req.Role != nil {
		targetRole = *req.Role
	}
	if targetRole == "admin" {
		if err := requireBoardRole(boardRole, "owner"); err != nil {
			return ErrInsufficientPermissions // admins cannot grant admin
		}
	}

	// Target must be a workspace member.
	board, err := s.boardRepo.FindByID(ctx, boardID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	if _, err = s.workspaceRepo.GetMember(ctx, board.WorkspaceID, req.UserID); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrUserNotFound // target is not a workspace member
		}
		return err
	}

	// Check if already a board member.
	alreadyMember, err := s.boardRepo.IsBoardMember(ctx, boardID, req.UserID)
	if err != nil {
		return err
	}
	if alreadyMember {
		return ErrAlreadyBoardMember
	}

	return s.boardRepo.AddBoardMember(ctx, boardID, req.UserID, targetRole)
}

// RemoveBoardMember removes an explicit board member.
// Requires board owner or admin.
// The board owner cannot be removed — ownership must be transferred first.
// Board admins can only remove editors — only the owner can remove an admin.
func (s *BoardService) RemoveBoardMember(ctx context.Context, boardID, targetUserID, requesterID, workspaceRole, boardRole string) error {
	if err := s.checkAccess(ctx, boardID, requesterID, workspaceRole); err != nil {
		return err
	}

	if err := requireBoardRole(boardRole, "owner", "admin"); err != nil {
		return err
	}

	// Look up the target's board role before removing.
	targetBoardRole, err := s.boardRepo.GetBoardMemberRole(ctx, boardID, targetUserID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	// Board owner cannot be removed.
	if targetBoardRole == "owner" {
		return ErrCannotRemoveBoardOwner
	}

	// Board admin cannot remove another admin — only the owner can.
	if targetBoardRole == "admin" && boardRole != "owner" {
		return ErrInsufficientPermissions
	}

	return s.boardRepo.RemoveBoardMember(ctx, boardID, targetUserID)
}

// UpdateBoardMemberRole changes the board role of an existing member.
// Only the board owner can change roles.
// To make someone the new owner, use TransferOwnership instead.
func (s *BoardService) UpdateBoardMemberRole(ctx context.Context, boardID, targetUserID, requesterID, workspaceRole, boardRole string, req models.UpdateBoardMemberRoleRequest) error {
	if err := s.checkAccess(ctx, boardID, requesterID, workspaceRole); err != nil {
		return err
	}

	// Only the board owner can change roles.
	if err := requireBoardRole(boardRole, "owner"); err != nil {
		return err
	}

	// Verify target is a board member.
	targetBoardRole, err := s.boardRepo.GetBoardMemberRole(ctx, boardID, targetUserID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	// Cannot change the owner's role via this endpoint — use TransferOwnership.
	if targetBoardRole == "owner" {
		return ErrCannotRemoveBoardOwner
	}

	return s.boardRepo.UpdateBoardMemberRole(ctx, boardID, targetUserID, req.Role)
}

// TransferOwnership designates a new board owner.
// Only the current board owner can call this.
// The new owner must already be a board member.
// After the transfer, the previous owner is downgraded to 'admin'.
func (s *BoardService) TransferOwnership(ctx context.Context, boardID, requesterID, workspaceRole, boardRole string, req models.TransferOwnershipRequest) error {
	if err := s.checkAccess(ctx, boardID, requesterID, workspaceRole); err != nil {
		return err
	}

	// Only the board owner can transfer ownership.
	if err := requireBoardRole(boardRole, "owner"); err != nil {
		return err
	}

	// Cannot transfer to yourself.
	if req.UserID == requesterID {
		return ErrInsufficientPermissions
	}

	// TransferOwnership in repo validates the target is already a board member.
	if err := s.boardRepo.TransferOwnership(ctx, boardID, requesterID, req.UserID); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrTargetNotBoardMember
		}
		return err
	}

	return nil
}

// ── Share link ────────────────────────────────────────────────────────────────

// GenerateShareLink creates a new public share token for a board.
// Requires board owner or admin.
func (s *BoardService) GenerateShareLink(ctx context.Context, boardID, userID, workspaceRole, boardRole string) (*models.ShareLinkResponse, error) {
	if err := s.checkAccess(ctx, boardID, userID, workspaceRole); err != nil {
		return nil, err
	}

	if err := requireBoardRole(boardRole, "owner", "admin"); err != nil {
		return nil, err
	}

	token, err := utils.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	if err := s.boardRepo.SetShareToken(ctx, boardID, &token); err != nil {
		return nil, err
	}

	return &models.ShareLinkResponse{
		URL:   s.cfg.FrontendURL + "/share/" + token,
		Token: token,
	}, nil
}

// RevokeShareLink sets the share token to NULL.
// Requires board owner or admin.
func (s *BoardService) RevokeShareLink(ctx context.Context, boardID, userID, workspaceRole, boardRole string) error {
	if err := s.checkAccess(ctx, boardID, userID, workspaceRole); err != nil {
		return err
	}

	if err := requireBoardRole(boardRole, "owner", "admin"); err != nil {
		return err
	}

	return s.boardRepo.SetShareToken(ctx, boardID, nil)
}

// GetPublicBoard returns a board via its share token. No auth required.
func (s *BoardService) GetPublicBoard(ctx context.Context, token string) (*models.BoardDetailResponse, error) {
	board, err := s.boardRepo.FindByShareToken(ctx, token)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	detail, err := s.boardRepo.GetBoardDetail(ctx, board.ID)
	if err != nil {
		return nil, err
	}

	detail.IsPublicView = true
	detail.MyBoardRole = "" // anonymous — no role

	return detail, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// checkAccess verifies the user can see this board at all.
// For workspace-visibility boards: any workspace member passes.
// For private boards: workspace owner/admin always pass; others must have an
// explicit board_members row.
func (s *BoardService) checkAccess(ctx context.Context, boardID, userID, workspaceRole string) error {
	board, err := s.boardRepo.FindByID(ctx, boardID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	if board.Visibility == "workspace" {
		return nil
	}

	// Private board — workspace owner/admin always have visibility.
	if workspaceRole == "owner" || workspaceRole == "admin" {
		return nil
	}

	// Private board — must have an explicit membership row.
	isMember, err := s.boardRepo.IsBoardMember(ctx, boardID, userID)
	if err != nil {
		return err
	}

	if !isMember {
		return ErrBoardAccessDenied
	}

	return nil
}