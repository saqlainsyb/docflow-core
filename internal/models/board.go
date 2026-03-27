package models

import "time"

// database row
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

// request DTOs
type CreateBoardRequest struct {
	Title      string  `json:"title"      binding:"required,min=1,max=100"`
	Visibility *string `json:"visibility" binding:"omitempty,oneof=workspace private"`
}

type UpdateBoardRequest struct {
	Title      *string `json:"title"      binding:"omitempty,min=1,max=100"`
	Visibility *string `json:"visibility" binding:"omitempty,oneof=workspace private"`
}

type AddBoardMemberRequest struct {
	UserID string `json:"user_id" binding:"required,uuid"`
}

// response DTOs
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
	ID           string               `json:"id"`
	WorkspaceID  string               `json:"workspace_id"`
	Title        string               `json:"title"`
	Visibility   string               `json:"visibility"`
	IsPublicView bool                 `json:"is_public_view"`
	Columns      []ColumnWithCards    `json:"columns"`
	Members      []MemberResponse     `json:"members"`
	CreatedAt    time.Time            `json:"created_at"`
}

type ShareLinkResponse struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}