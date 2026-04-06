// internal/repositories/invitations.go
package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saqlainsyb/docflow-core/internal/models"
)

type InvitationRepository struct {
	db *pgxpool.Pool
}

func NewInvitationRepository(db *pgxpool.Pool) *InvitationRepository {
	return &InvitationRepository{db: db}
}

// Create inserts a new pending invitation.
// tokenHash must be the SHA-256 hex of the raw token — never store raw tokens.
// expiresAt is set by the service layer (typically now + 7 days).
func (r *InvitationRepository) Create(
	ctx context.Context,
	workspaceID, invitedEmail, tokenHash, invitedBy, role string,
	expiresAt time.Time,
) (*models.WorkspaceInvitation, error) {
	inv := &models.WorkspaceInvitation{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO workspace_invitations
			(workspace_id, invited_email, token_hash, invited_by, role, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, workspace_id, invited_email, token_hash, invited_by,
		          role, status, expires_at, accepted_at, created_at
	`, workspaceID, invitedEmail, tokenHash, invitedBy, role, expiresAt).Scan(
		&inv.ID,
		&inv.WorkspaceID,
		&inv.InvitedEmail,
		&inv.TokenHash,
		&inv.InvitedBy,
		&inv.Role,
		&inv.Status,
		&inv.ExpiresAt,
		&inv.AcceptedAt,
		&inv.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return inv, nil
}

// FindByTokenHash looks up an invitation by the SHA-256 hash of the raw token.
// This is the hot path — called on every accept click. The unique index on
// token_hash makes it an O(1) lookup.
// Returns ErrNotFound if the token does not exist.
func (r *InvitationRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*models.WorkspaceInvitation, error) {
	inv := &models.WorkspaceInvitation{}
	err := r.db.QueryRow(ctx, `
		SELECT id, workspace_id, invited_email, token_hash, invited_by,
		       role, status, expires_at, accepted_at, created_at
		FROM workspace_invitations
		WHERE token_hash = $1
	`, tokenHash).Scan(
		&inv.ID,
		&inv.WorkspaceID,
		&inv.InvitedEmail,
		&inv.TokenHash,
		&inv.InvitedBy,
		&inv.Role,
		&inv.Status,
		&inv.ExpiresAt,
		&inv.AcceptedAt,
		&inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return inv, nil
}

// FindByID looks up an invitation by its primary key UUID.
// Used for cancel operations where the frontend sends the invitation ID.
func (r *InvitationRepository) FindByID(ctx context.Context, id string) (*models.WorkspaceInvitation, error) {
	inv := &models.WorkspaceInvitation{}
	err := r.db.QueryRow(ctx, `
		SELECT id, workspace_id, invited_email, token_hash, invited_by,
		       role, status, expires_at, accepted_at, created_at
		FROM workspace_invitations
		WHERE id = $1
	`, id).Scan(
		&inv.ID,
		&inv.WorkspaceID,
		&inv.InvitedEmail,
		&inv.TokenHash,
		&inv.InvitedBy,
		&inv.Role,
		&inv.Status,
		&inv.ExpiresAt,
		&inv.AcceptedAt,
		&inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return inv, nil
}

// ListPending returns all pending (non-expired) invitations for a workspace,
// joined with the inviter's display name for the management UI.
// Ordered newest-first so recently sent invites appear at the top.
func (r *InvitationRepository) ListPending(ctx context.Context, workspaceID string) ([]models.PendingInvitationResponse, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			wi.id,
			wi.invited_email,
			u.name  AS inviter_name,
			wi.role,
			wi.expires_at,
			wi.created_at
		FROM workspace_invitations wi
		JOIN users u ON u.id = wi.invited_by
		WHERE wi.workspace_id = $1
		  AND wi.status = 'pending'
		  AND wi.expires_at > NOW()
		ORDER BY wi.created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.PendingInvitationResponse
	for rows.Next() {
		var p models.PendingInvitationResponse
		if err := rows.Scan(
			&p.ID,
			&p.InvitedEmail,
			&p.InviterName,
			&p.Role,
			&p.ExpiresAt,
			&p.CreatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// GetDetailByTokenHash fetches the invitation with its workspace name and
// inviter name in a single query — used by the public accept page endpoint.
func (r *InvitationRepository) GetDetailByTokenHash(ctx context.Context, tokenHash string) (*models.InvitationDetailResponse, error) {
	detail := &models.InvitationDetailResponse{}
	err := r.db.QueryRow(ctx, `
		SELECT
			wi.id,
			wi.workspace_id,
			w.name   AS workspace_name,
			wi.invited_email,
			u.name   AS inviter_name,
			wi.role,
			wi.expires_at
		FROM workspace_invitations wi
		JOIN workspaces w ON w.id = wi.workspace_id
		JOIN users u      ON u.id = wi.invited_by
		WHERE wi.token_hash = $1
	`, tokenHash).Scan(
		&detail.ID,
		&detail.WorkspaceID,
		&detail.WorkspaceName,
		&detail.InvitedEmail,
		&detail.InviterName,
		&detail.Role,
		&detail.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return detail, nil
}

// Accept marks the invitation as accepted and records the timestamp.
// This is called from AcceptInvitation after the workspace member row is inserted.
// Both operations (AddMember + Accept) must run in the same transaction —
// see InvitationService.AcceptInvitation for the transaction management.
func (r *InvitationRepository) Accept(ctx context.Context, id string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE workspace_invitations
		SET status = 'accepted', accepted_at = NOW()
		WHERE id = $1 AND status = 'pending'
	`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Cancel marks the invitation as cancelled.
// Cancelled invitations cannot be accepted — the accept handler rejects them.
// The partial unique index (WHERE status = 'pending') means a fresh invite
// can be sent to the same email after cancellation.
func (r *InvitationRepository) Cancel(ctx context.Context, id string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE workspace_invitations
		SET status = 'cancelled'
		WHERE id = $1 AND status = 'pending'
	`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// HasPendingForEmail returns true if there is already a non-expired pending
// invitation for this email in this workspace.
// Used before creating a new invitation to surface a clean error to the requester.
func (r *InvitationRepository) HasPendingForEmail(ctx context.Context, workspaceID, email string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM workspace_invitations
			WHERE workspace_id = $1
			  AND invited_email = $2
			  AND status = 'pending'
			  AND expires_at > NOW()
		)
	`, workspaceID, email).Scan(&exists)
	return exists, err
}