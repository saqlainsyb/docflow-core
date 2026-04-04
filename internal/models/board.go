package models

import "time"

// ── Database row ──────────────────────────────────────────────────────────────

type Board struct {
	ID          string    `db:"id"`
	WorkspaceID string    `db:"workspace_id"`
	Title       string    `db:"title"`
	Visibility  string    `db:"visibility"`
	ShareToken  *string   `db:"share_token"`
	CreatedBy   *string   `db:"created_by"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

// ── Request DTOs ──────────────────────────────────────────────────────────────

type CreateBoardRequest struct {
	Title      string  `json:"title"      binding:"required,min=1,max=100"`
	Visibility *string `json:"visibility" binding:"omitempty,oneof=workspace private"`
}

type UpdateBoardRequest struct {
	Title      *string `json:"title"      binding:"omitempty,min=1,max=100"`
	Visibility *string `json:"visibility" binding:"omitempty,oneof=workspace private"`
}

// AddBoardMemberRequest adds a user to a board with an optional board role.
// Role defaults to "editor" if omitted.
type AddBoardMemberRequest struct {
	UserID string  `json:"user_id" binding:"required,uuid"`
	Role   *string `json:"role"    binding:"omitempty,oneof=admin editor"`
	// "owner" is intentionally excluded — ownership is transferred, not assigned.
}

// UpdateBoardMemberRoleRequest changes the board role of an existing member.
// Only the board owner can call this.
type UpdateBoardMemberRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=admin editor"`
	// "owner" is intentionally excluded — use the transfer-ownership endpoint.
}

// TransferOwnershipRequest designates a new board owner.
// The current owner is downgraded to "admin" automatically.
type TransferOwnershipRequest struct {
	UserID string `json:"user_id" binding:"required,uuid"`
}

// ── Response DTOs ─────────────────────────────────────────────────────────────

type BoardResponse struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Title       string    `json:"title"`
	Visibility  string    `json:"visibility"`
	MemberCount int       `json:"member_count"`
	CardCount   int       `json:"card_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type BoardDetailResponse struct {
	ID           string            `json:"id"`
	WorkspaceID  string            `json:"workspace_id"`
	Title        string            `json:"title"`
	Visibility   string            `json:"visibility"`
	IsPublicView bool              `json:"is_public_view"`
	MyBoardRole  string            `json:"my_board_role"` // caller's resolved board role
	Columns      []ColumnWithCards `json:"columns"`
	Members      []BoardMember     `json:"members"`
	CreatedAt    time.Time         `json:"created_at"`
}

// BoardMember is returned in board detail and member-list responses.
// Role is the board-level role (owner / admin / editor) — NOT the workspace role.
type BoardMember struct {
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	AvatarURL *string   `json:"avatar_url"`
	BoardRole string    `json:"board_role"`
	AddedAt   time.Time `json:"added_at"`
}

type ShareLinkResponse struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}