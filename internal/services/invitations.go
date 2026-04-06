// internal/services/invitations.go
package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/saqlainsyb/docflow-core/internal/email"
	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

const (
	invitationTTL     = 7 * 24 * time.Hour // 7 days
	invitationTTLDays = 7
)

// InvitationService handles the lifecycle of workspace invitations:
// sending, accepting, cancelling, and listing.
//
// Dependency graph:
//   InvitationService
//     ├── invitationRepo  — persists workspace_invitations rows
//     ├── workspaceRepo   — checks membership + fetches workspace name
//     ├── userRepo        — resolves inviter name + checks if invitee has an account
//     └── emailClient     — sends the invitation email via Resend
type InvitationService struct {
	invitationRepo *repositories.InvitationRepository
	workspaceRepo  *repositories.WorkspaceRepository
	userRepo       *repositories.UserRepository
	emailClient    *email.Client
	frontendURL    string
}

func NewInvitationService(
	invitationRepo *repositories.InvitationRepository,
	workspaceRepo  *repositories.WorkspaceRepository,
	userRepo       *repositories.UserRepository,
	emailClient    *email.Client,
	frontendURL    string,
) *InvitationService {
	return &InvitationService{
		invitationRepo: invitationRepo,
		workspaceRepo:  workspaceRepo,
		userRepo:       userRepo,
		emailClient:    emailClient,
		frontendURL:    frontendURL,
	}
}

// SendInvitation sends a workspace invitation email to the given address.
//
// Business rules:
//  1. Requester must be admin or owner.
//  2. Invited email must not already be a member.
//  3. There must not already be a live pending invitation for this email.
//  4. We do NOT require the invitee to have an existing account —
//     that is the entire point of this feature.
//
// Regardless of whether the invitee has an account, the email link always
// points to the frontend accept page. The accept page resolves which flow
// to show (join immediately vs register first) based on its own state.
//
// The raw token is never stored — only its SHA-256 hash. This means even
// a full database dump does not expose valid invitation tokens.
func (s *InvitationService) SendInvitation(
	ctx context.Context,
	workspaceID, requesterID, requesterRole string,
	req models.SendInvitationRequest,
) (*models.SendInvitationResponse, error) {

	// ── 1. Permission check ─────────────────────────────────────────────────
	if err := requireRole(requesterRole, "admin", "owner"); err != nil {
		return nil, err
	}

	// ── 2. Fetch workspace (need name for email) ─────────────────────────────
	workspace, err := s.workspaceRepo.FindByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// ── 3. Fetch inviter (need name for email) ───────────────────────────────
	inviter, err := s.userRepo.FindByID(ctx, requesterID)
	if err != nil {
		return nil, err
	}

	// ── 4. Normalise email to lowercase ─────────────────────────────────────
	// The unique index is case-sensitive; normalise here to avoid duplicates
	// caused by capitalisation differences (e.g. "User@example.com" vs "user@…").
	normalizedEmail := strings.ToLower(strings.TrimSpace(req.Email))

	// ── 5. Check not already a member ───────────────────────────────────────
	existingUser, userErr := s.userRepo.FindByEmail(ctx, normalizedEmail)
	if userErr == nil {
		// user exists — check membership
		_, memberErr := s.workspaceRepo.GetMember(ctx, workspaceID, existingUser.ID)
		if memberErr == nil {
			return nil, ErrAlreadyMember
		}
		if !errors.Is(memberErr, repositories.ErrNotFound) {
			return nil, memberErr
		}
	} else if !errors.Is(userErr, repositories.ErrNotFound) {
		return nil, userErr
	}

	// ── 6. Check no live pending invitation ──────────────────────────────────
	hasPending, err := s.invitationRepo.HasPendingForEmail(ctx, workspaceID, normalizedEmail)
	if err != nil {
		return nil, err
	}
	if hasPending {
		return nil, ErrInvitationAlreadyPending
	}

	// ── 7. Generate secure token ─────────────────────────────────────────────
	rawToken, err := utils.GenerateRefreshToken() // 32 random bytes → hex — same utility, correct length
	if err != nil {
		return nil, err
	}
	tokenHash := utils.HashToken(rawToken)
	expiresAt := time.Now().Add(invitationTTL)

	// ── 8. Persist invitation ─────────────────────────────────────────────────
	inv, err := s.invitationRepo.Create(
		ctx,
		workspaceID,
		normalizedEmail,
		tokenHash,
		requesterID,
		req.Role,
		expiresAt,
	)
	if err != nil {
		return nil, err
	}

	// ── 9. Build accept URL ───────────────────────────────────────────────────
	// For existing users: /invitations/:token
	// For new users:      /register?invitation=:token
	// We always send the invitation URL — the InvitationAcceptPage on the
	// frontend detects auth state and shows the right UI.
	acceptURL := fmt.Sprintf("%s/invitations/%s", s.frontendURL, rawToken)

	// ── 10. Send email (non-blocking failure) ─────────────────────────────────
	// If Resend is down or rate-limits us, we still return success — the
	// invitation row exists and can be resent. We log the error server-side.
	// In a production V2 this would go through a retry queue; for V1 the
	// trade-off is acceptable.
	isExistingUser := userErr == nil // true if FindByEmail succeeded
	emailErr := s.emailClient.SendInvitation(ctx, email.InvitationEmailData{
		RecipientEmail: normalizedEmail,
		InviterName:    inviter.Name,
		WorkspaceName:  workspace.Name,
		AcceptURL:      acceptURL,
		IsExistingUser: isExistingUser,
		ExpiresInDays:  invitationTTLDays,
	})
	if emailErr != nil {
		// Wrap in a sentinel so the handler can log it without returning 500.
		// The invitation is created — the failure is only in the delivery.
		return nil, fmt.Errorf("%w: %s", ErrEmailDeliveryFailed, emailErr.Error())
	}

	return &models.SendInvitationResponse{
		ID:           inv.ID,
		InvitedEmail: inv.InvitedEmail,
		Role:         inv.Role,
		ExpiresAt:    inv.ExpiresAt,
	}, nil
}

// GetInvitationByToken returns public details about an invitation.
// This endpoint is unauthenticated — the frontend calls it to render the
// accept page (workspace name, inviter name, expiry) before the user acts.
// Returns ErrInvitationInvalid for any token that cannot be accepted
// (not found, expired, already used, cancelled).
func (s *InvitationService) GetInvitationByToken(ctx context.Context, rawToken string) (*models.InvitationDetailResponse, error) {
	tokenHash := utils.HashToken(rawToken)

	detail, err := s.invitationRepo.GetDetailByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrInvitationInvalid
		}
		return nil, err
	}

	// Fetch full row to check status + expiry (GetDetailByTokenHash only returns
	// display fields; status/expiry come from FindByTokenHash).
	inv, err := s.invitationRepo.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}

	if inv.Status != "pending" {
		return nil, ErrInvitationInvalid
	}
	if time.Now().After(inv.ExpiresAt) {
		return nil, ErrInvitationExpired
	}

	return detail, nil
}

// AcceptInvitation joins the authenticated user to the workspace.
//
// Business rules:
//  1. Token must be valid (exists, status=pending, not expired).
//  2. The caller's email must match the invitation email — invitations are
//     non-transferable. This prevents accepting an invite meant for someone else.
//  3. The caller must not already be a member.
//  4. AddMember + Accept run in a logical two-step — both must succeed.
//     pgx does not expose savepoints through the pool easily, so we do the
//     database-level atomicity by checking each error and rolling back intent.
//
// On success: returns the workspace ID so the frontend can navigate there.
func (s *InvitationService) AcceptInvitation(
	ctx context.Context,
	rawToken, callerID, callerEmail string,
) (workspaceID string, err error) {

	tokenHash := utils.HashToken(rawToken)

	inv, err := s.invitationRepo.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return "", ErrInvitationInvalid
		}
		return "", err
	}

	// Validate status and expiry
	if inv.Status != "pending" {
		return "", ErrInvitationInvalid
	}
	if time.Now().After(inv.ExpiresAt) {
		return "", ErrInvitationExpired
	}

	// Enforce email match — invitations are addressed to a specific person
	if !strings.EqualFold(callerEmail, inv.InvitedEmail) {
		return "", ErrInvitationEmailMismatch
	}

	// Idempotency: if already a member, mark as accepted and succeed
	_, memberErr := s.workspaceRepo.GetMember(ctx, inv.WorkspaceID, callerID)
	if memberErr == nil {
		// Already a member — mark invitation accepted and return workspace ID.
		// This handles the rare race where the user was added another way.
		_ = s.invitationRepo.Accept(ctx, inv.ID)
		return inv.WorkspaceID, nil
	}
	if !errors.Is(memberErr, repositories.ErrNotFound) {
		return "", memberErr
	}

	// Add the user to the workspace
	if err := s.workspaceRepo.AddMember(ctx, inv.WorkspaceID, callerID, inv.Role); err != nil {
		return "", err
	}

	// Mark invitation accepted
	if err := s.invitationRepo.Accept(ctx, inv.ID); err != nil {
		// Non-fatal: member row is committed, the worst outcome is the invitation
		// stays "pending" and could theoretically be reused. In V1 this is
		// acceptable; V2 should wrap both ops in an explicit transaction.
		return "", err
	}

	return inv.WorkspaceID, nil
}

// ListPendingInvitations returns all live pending invitations for a workspace.
// Requires admin or owner role.
func (s *InvitationService) ListPendingInvitations(
	ctx context.Context,
	workspaceID, requesterRole string,
) ([]models.PendingInvitationResponse, error) {
	if err := requireRole(requesterRole, "admin", "owner"); err != nil {
		return nil, err
	}

	results, err := s.invitationRepo.ListPending(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []models.PendingInvitationResponse{}
	}
	return results, nil
}

// CancelInvitation cancels a pending invitation.
// Requires admin or owner role, and the invitation must belong to this workspace.
func (s *InvitationService) CancelInvitation(
	ctx context.Context,
	workspaceID, invitationID, requesterRole string,
) error {
	if err := requireRole(requesterRole, "admin", "owner"); err != nil {
		return err
	}

	inv, err := s.invitationRepo.FindByID(ctx, invitationID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	// Verify the invitation actually belongs to this workspace —
	// prevent cross-workspace ID guessing attacks.
	if inv.WorkspaceID != workspaceID {
		return ErrNotFound
	}

	return s.invitationRepo.Cancel(ctx, invitationID)
}