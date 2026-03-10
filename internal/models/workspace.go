package models

import "time"

// database rows
type Workspace struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	OwnerID   string    `db:"owner_id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type WorkspaceMember struct {
	WorkspaceID string    `db:"workspace_id"`
	UserID      string    `db:"user_id"`
	Role        string    `db:"role"`
	JoinedAt    time.Time `db:"joined_at"`
}

// request DTOs
type CreateWorkspaceRequest struct {
	Name string `json:"name" binding:"required,min=2,max=100"`
}

type UpdateWorkspaceRequest struct {
	Name *string `json:"name" binding:"omitempty,min=2,max=100"`
}

type InviteMemberRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type UpdateMemberRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=admin member"`
}

// response DTOs
type WorkspaceResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	OwnerID     string    `json:"owner_id"`
	MemberCount int       `json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type MemberResponse struct {
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	AvatarURL *string   `json:"avatar_url"`
	Role      string    `json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
}

type WorkspaceDetailResponse struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	OwnerID   string           `json:"owner_id"`
	Members   []MemberResponse `json:"members"`
	CreatedAt time.Time        `json:"created_at"`
}