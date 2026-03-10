package models

import "time"

// database row
type Card struct {
	ID         string    `db:"id"`
	BoardID    string    `db:"board_id"`
	ColumnID   string    `db:"column_id"`
	Title      string    `db:"title"`
	Position   float64   `db:"position"`
	Color      *string   `db:"color"`
	AssigneeID *string   `db:"assignee_id"`
	Archived   bool      `db:"archived"`
	CreatedBy  *string   `db:"created_by"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

// request DTOs
type CreateCardRequest struct {
	Title string  `json:"title" binding:"required,min=1,max=200"`
	Color *string `json:"color" binding:"omitempty"`
}

type UpdateCardRequest struct {
	Title      *string `json:"title"       binding:"omitempty,min=1,max=200"`
	Color      *string `json:"color"       binding:"omitempty"`
	AssigneeID *string `json:"assignee_id" binding:"omitempty,uuid"`
}

type MoveCardRequest struct {
	ColumnID string  `json:"column_id" binding:"required,uuid"`
	Position float64 `json:"position"  binding:"required,gt=0"`
}

// response DTO
type CardResponse struct {
	ID         string      `json:"id"`
	BoardID    string      `json:"board_id"`
	ColumnID   string      `json:"column_id"`
	Title      string      `json:"title"`
	Position   float64     `json:"position"`
	Color      *string     `json:"color"`
	Assignee   *UserPublic `json:"assignee"`
	DocumentID string      `json:"document_id"`
	Archived   bool        `json:"archived"`
	CreatedAt  time.Time   `json:"created_at"`
}