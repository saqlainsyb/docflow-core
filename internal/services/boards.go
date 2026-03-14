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

// ListBoards returns all boards in a workspace visible to the requesting user.
// Visibility filtering is handled in the repository query itself.
func (s *BoardService) ListBoards(ctx context.Context, workspaceID, userID string) ([]models.BoardResponse, error) {
	return s.boardRepo.FindByWorkspace(ctx, workspaceID, userID)
}

// CreateBoard creates a new board in a workspace.
// Any workspace member can create a board.
// If visibility is private, the creator is automatically added to board_members.
func (s *BoardService) CreateBoard(ctx context.Context, workspaceID, userID string, req models.CreateBoardRequest) (*models.Board, error) {
	visibility := "workspace"
	if req.Visibility != nil {
		visibility = *req.Visibility
	}

	if visibility == "private" {
		return s.boardRepo.CreatePrivate(ctx, workspaceID, req.Title, userID)
	}

	return s.boardRepo.Create(ctx, workspaceID, req.Title, visibility, userID)
}

// GetBoardDetail returns the full board with nested columns and cards.
// Performs an access check before returning data.
func (s *BoardService) GetBoardDetail(ctx context.Context, boardID, userID, memberRole string) (*models.BoardDetailResponse, error) {
	if err := s.checkAccess(ctx, boardID, userID, memberRole); err != nil {
		return nil, err
	}

	return s.boardRepo.GetBoardDetail(ctx, boardID)
}

// UpdateBoard changes a board's title and/or visibility.
// Requires board access — any member with access can update.
func (s *BoardService) UpdateBoard(ctx context.Context, boardID, userID, memberRole string, req models.UpdateBoardRequest) (*models.Board, error) {
	if err := s.checkAccess(ctx, boardID, userID, memberRole); err != nil {
		return nil, err
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
// Requires workspace owner role.
func (s *BoardService) DeleteBoard(ctx context.Context, boardID, userID, memberRole string) error {
	if err := requireRole(memberRole, "owner"); err != nil {
		return err
	}

	if err := s.checkAccess(ctx, boardID, userID, memberRole); err != nil {
		return err
	}

	if err := s.boardRepo.Delete(ctx, boardID); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	return nil
}

// ListBoardMembers returns explicit members of a board.
// Only meaningful for private boards — workspace boards don't use board_members.
func (s *BoardService) ListBoardMembers(ctx context.Context, boardID, userID, memberRole string) ([]models.MemberResponse, error) {
	if err := s.checkAccess(ctx, boardID, userID, memberRole); err != nil {
		return nil, err
	}

	board, err := s.boardRepo.FindByID(ctx, boardID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return s.boardRepo.ListBoardMembers(ctx, boardID, board.WorkspaceID)
}

// AddBoardMember adds an explicit board member.
// Requires admin or owner workspace role.
func (s *BoardService) AddBoardMember(ctx context.Context, boardID, userID, memberRole string, req models.AddBoardMemberRequest) error {
	if err := requireRole(memberRole, "admin", "owner"); err != nil {
		return err
	}

	if err := s.checkAccess(ctx, boardID, userID, memberRole); err != nil {
		return err
	}

	// verify the target user is a workspace member first
	board, err := s.boardRepo.FindByID(ctx, boardID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	_, err = s.workspaceRepo.GetMember(ctx, board.WorkspaceID, req.UserID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	return s.boardRepo.AddBoardMember(ctx, boardID, req.UserID)
}

// RemoveBoardMember removes an explicit board member.
// Requires admin or owner workspace role.
func (s *BoardService) RemoveBoardMember(ctx context.Context, boardID, targetUserID, requesterID, memberRole string) error {
	if err := requireRole(memberRole, "admin", "owner"); err != nil {
		return err
	}

	if err := s.checkAccess(ctx, boardID, requesterID, memberRole); err != nil {
		return err
	}

	if err := s.boardRepo.RemoveBoardMember(ctx, boardID, targetUserID); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	return nil
}

// GenerateShareLink creates a new public share token for a board.
// Requires admin or owner workspace role.
// Overwrites any existing token — old links stop working immediately.
func (s *BoardService) GenerateShareLink(ctx context.Context, boardID, userID, memberRole string) (*models.ShareLinkResponse, error) {
	if err := requireRole(memberRole, "admin", "owner"); err != nil {
		return nil, err
	}

	if err := s.checkAccess(ctx, boardID, userID, memberRole); err != nil {
		return nil, err
	}

	// 32 cryptographically random bytes encoded as hex
	token, err := utils.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	if err := s.boardRepo.SetShareToken(ctx, boardID, &token); err != nil {
		return nil, err
	}

	return &models.ShareLinkResponse{
		URL:   s.cfg.AppURL + "/api/v1/share/" + token,
		Token: token,
	}, nil
}

// RevokeShareLink sets the share token to NULL.
// Any request using the old token now returns 404 immediately.
// Requires admin or owner workspace role.
func (s *BoardService) RevokeShareLink(ctx context.Context, boardID, userID, memberRole string) error {
	if err := requireRole(memberRole, "admin", "owner"); err != nil {
		return err
	}

	if err := s.checkAccess(ctx, boardID, userID, memberRole); err != nil {
		return err
	}

	return s.boardRepo.SetShareToken(ctx, boardID, nil)
}

// GetPublicBoard returns a board via its share token.
// No authentication required — used for the public read-only view.
// Sets IsPublicView: true on the response so the frontend disables writes.
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

	return detail, nil
}

// checkAccess verifies the user can access the given board.
// Rules from the architecture doc:
// - workspace visibility: any workspace member has access
// - private: only explicit board_members, OR workspace owner/admin
// Returns ErrBoardAccessDenied if the user has no access.
func (s *BoardService) checkAccess(ctx context.Context, boardID, userID, memberRole string) error {
	board, err := s.boardRepo.FindByID(ctx, boardID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	// workspace-visibility boards: any workspace member can access
	if board.Visibility == "workspace" {
		return nil
	}

	// private boards: owners and admins always pass
	if memberRole == "owner" || memberRole == "admin" {
		return nil
	}

	// private boards: check explicit board membership
	isMember, err := s.boardRepo.IsBoardMember(ctx, boardID, userID)
	if err != nil {
		return err
	}

	if !isMember {
		return ErrBoardAccessDenied
	}

	return nil
}