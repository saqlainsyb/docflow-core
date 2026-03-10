package models

import "time"

// database rows
type Document struct {
	ID            string    `db:"id"`
	CardID        string    `db:"card_id"`
	Snapshot      []byte    `db:"snapshot"`
	SnapshotClock int       `db:"snapshot_clock"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

type DocumentUpdate struct {
	ID         int64     `db:"id"`
	DocumentID string    `db:"document_id"`
	UpdateData []byte    `db:"update_data"`
	Clock      int       `db:"clock"`
	CreatedAt  time.Time `db:"created_at"`
}

// response DTOs
type DocumentTokenResponse struct {
	Token      string `json:"token"`
	DocumentID string `json:"document_id"`
	Color      string `json:"color"`
	ExpiresIn  int    `json:"expires_in"`
}

type DocumentSnapshotResponse struct {
	DocumentID string `json:"document_id"`
	Snapshot   string `json:"snapshot"`
	Clock      int    `json:"clock"`
}