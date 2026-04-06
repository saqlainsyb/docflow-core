// internal/models/invitation.go
package models

import "time"

// WorkspaceInvitation is the database row shape for workspace_invitations.
type WorkspaceInvitation struct {
	ID            string     `db:"id"`
	WorkspaceID   string     `db:"workspace_id"`
	InvitedEmail  string     `db:"invited_email"`
	TokenHash     string     `db:"token_hash"`
	InvitedBy     string     `db:"invited_by"`
	Role          string     `db:"role"`
	Status        string     `db:"status"`
	ExpiresAt     time.Time  `db:"expires_at"`
	AcceptedAt    *time.Time `db:"accepted_at"`
	CreatedAt     time.Time  `db:"created_at"`
}

// ── Request DTOs ─────────────────────────────────────────────────────────────

// SendInvitationRequest is the body for POST /workspaces/:id/invitations.
type SendInvitationRequest struct {
	Email string `json:"email" binding:"required,email"`
	Role  string `json:"role"  binding:"required,oneof=admin member"`
}

// ── Response DTOs ─────────────────────────────────────────────────────────────

// PendingInvitationResponse is returned in the workspace invitation list.
// Exposes enough info for the management UI without leaking the token.
type PendingInvitationResponse struct {
	ID            string    `json:"id"`
	InvitedEmail  string    `json:"invited_email"`
	InviterName   string    `json:"inviter_name"`
	Role          string    `json:"role"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// InvitationDetailResponse is returned by the public GET /invitations/:token
// endpoint. The frontend uses this to render the accept page before the user
// clicks the CTA. We deliberately omit sensitive fields (token_hash, invited_by UUID).
type InvitationDetailResponse struct {
	ID            string    `json:"id"`
	WorkspaceID   string    `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
	InvitedEmail  string    `json:"invited_email"`
	InviterName   string    `json:"inviter_name"`
	Role          string    `json:"role"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// SendInvitationResponse is returned after a successful invite send.
type SendInvitationResponse struct {
	ID           string    `json:"id"`
	InvitedEmail string    `json:"invited_email"`
	Role         string    `json:"role"`
	ExpiresAt    time.Time `json:"expires_at"`
}