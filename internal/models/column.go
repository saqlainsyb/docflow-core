package models

import "time"

// database row
type Column struct {
	ID        string    `db:"id"`
	BoardID   string    `db:"board_id"`
	Title     string    `db:"title"`
	Position  float64   `db:"position"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// request DTOs
type CreateColumnRequest struct {
	Title string `json:"title" binding:"required,min=1,max=100"`
}

type UpdateColumnRequest struct {
	Title    *string  `json:"title"    binding:"omitempty,min=1,max=100"`
	Position *float64 `json:"position" binding:"omitempty,gt=0"`
}

// response DTOs
type ColumnResponse struct {
	ID        string    `json:"id"`
	BoardID   string    `json:"board_id"`
	Title     string    `json:"title"`
	Position  float64   `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

// used inside BoardDetailResponse
type ColumnWithCards struct {
	ID        string         `json:"id"`
	BoardID   string         `json:"board_id"`
	Title     string         `json:"title"`
	Position  float64        `json:"position"`
	Cards     []CardResponse `json:"cards"`
	CreatedAt time.Time      `json:"created_at"`
}