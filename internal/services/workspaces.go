package services

import (
	"context"
	"errors"

	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
)

type WorkspaceService struct {
	workspaceRepo *repositories.WorkspaceRepository
	userRepo      *repositories.UserRepository
}

func NewWorkspaceService(
	workspaceRepo *repositories.WorkspaceRepository,
	userRepo *repositories.UserRepository,
) *WorkspaceService {
	return &WorkspaceService{
		workspaceRepo: workspaceRepo,
		userRepo:      userRepo,
	}
}

// CreateWorkspace creates a new workspace owned by the requesting user.
// The owner is automatically added to workspace_members with role 'owner'.
func (s *WorkspaceService) CreateWorkspace(ctx context.Context, userID string, req models.CreateWorkspaceRequest) (*models.Workspace, error) {
	return s.workspaceRepo.CreateWithOwner(ctx, req.Name, userID)
}

// ListWorkspaces returns all workspaces the user belongs to in any role.
func (s *WorkspaceService) ListWorkspaces(ctx context.Context, userID string) ([]models.WorkspaceResponse, error) {
	return s.workspaceRepo.FindByUserID(ctx, userID)
}

// GetWorkspace returns workspace detail with full member list.
// The membership check is done upstream by the workspace middleware —
// by the time this is called we already know the user belongs here.
func (s *WorkspaceService) GetWorkspace(ctx context.Context, workspaceID string) (*models.WorkspaceDetailResponse, error) {
	ws, err := s.workspaceRepo.FindByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	members, err := s.workspaceRepo.ListMembers(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	return &models.WorkspaceDetailResponse{
		ID:        ws.ID,
		Name:      ws.Name,
		OwnerID:   ws.OwnerID,
		Members:   members,
		CreatedAt: ws.CreatedAt,
	}, nil
}

// RenameWorkspace updates the workspace name.
// Requires admin or owner role.
func (s *WorkspaceService) RenameWorkspace(ctx context.Context, workspaceID, requesterRole string, req models.UpdateWorkspaceRequest) (*models.Workspace, error) {
	if err := requireRole(requesterRole, "admin", "owner"); err != nil {
		return nil, err
	}

	ws, err := s.workspaceRepo.Update(ctx, workspaceID, *req.Name)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return ws, nil
}

// DeleteWorkspace permanently removes the workspace and everything in it.
// Cascading FK constraints in the schema handle all child records.
// Requires owner role — only the owner can delete their workspace.
func (s *WorkspaceService) DeleteWorkspace(ctx context.Context, workspaceID, requesterRole string) error {
	if err := requireRole(requesterRole, "owner"); err != nil {
		return err
	}

	if err := s.workspaceRepo.Delete(ctx, workspaceID); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	return nil
}

// ListMembers returns all members of a workspace.
func (s *WorkspaceService) ListMembers(ctx context.Context, workspaceID string) ([]models.MemberResponse, error) {
	return s.workspaceRepo.ListMembers(ctx, workspaceID)
}

// InviteMember adds an existing user to the workspace with role 'member'.
// Rules:
// - Requester must be admin or owner
// - Invited user must already have an account (V1 constraint — no email invites)
// - User must not already be a member
func (s *WorkspaceService) InviteMember(ctx context.Context, workspaceID, requesterRole string, req models.InviteMemberRequest) error {
	if err := requireRole(requesterRole, "admin", "owner"); err != nil {
		return err
	}

	// user must already have an account
	user, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	// check not already a member
	_, err = s.workspaceRepo.GetMember(ctx, workspaceID, user.ID)
	if err == nil {
		// member row found — already a member
		return ErrAlreadyMember
	}
	if !errors.Is(err, repositories.ErrNotFound) {
		return err
	}

	return s.workspaceRepo.AddMember(ctx, workspaceID, user.ID, "member")
}

// RemoveMember removes a user from the workspace.
// Rules:
// - Requester must be admin or owner
// - Cannot remove the workspace owner (they must delete the workspace instead)
// - An admin cannot remove another admin — only the owner can
func (s *WorkspaceService) RemoveMember(ctx context.Context, workspaceID, requesterID, requesterRole, targetUserID string) error {
	if err := requireRole(requesterRole, "admin", "owner"); err != nil {
		return err
	}

	// look up the target member to check their role
	target, err := s.workspaceRepo.GetMember(ctx, workspaceID, targetUserID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	// cannot remove the workspace owner
	if target.Role == "owner" {
		return ErrCannotRemoveOwner
	}

	// admins cannot remove other admins — only the owner can
	if requesterRole == "admin" && target.Role == "admin" {
		return ErrInsufficientPermissions
	}

	return s.workspaceRepo.RemoveMember(ctx, workspaceID, targetUserID)
}

// UpdateMemberRole changes a member's role.
// Rules:
// - Requester must be owner
// - Cannot change own role
// - Target role must be 'admin' or 'member' — 'owner' cannot be assigned here
func (s *WorkspaceService) UpdateMemberRole(ctx context.Context, workspaceID, requesterID, requesterRole, targetUserID string, req models.UpdateMemberRoleRequest) error {
	if err := requireRole(requesterRole, "owner"); err != nil {
		return err
	}

	// cannot change your own role
	if requesterID == targetUserID {
		return ErrCannotChangeSelfRole
	}

	// verify target is actually a member
	_, err := s.workspaceRepo.GetMember(ctx, workspaceID, targetUserID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	return s.workspaceRepo.UpdateMemberRole(ctx, workspaceID, targetUserID, req.Role)
}

// requireRole checks that the requester holds one of the permitted roles.
// Returns ErrInsufficientPermissions if none match.
func requireRole(actual string, permitted ...string) error {
	for _, r := range permitted {
		if actual == r {
			return nil
		}
	}
	return ErrInsufficientPermissions
}