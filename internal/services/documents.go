package services

import (
	"context"
	"encoding/base64"
	"errors"

	"github.com/saqlainsyb/docflow-core/internal/config"
	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

// cursorColors is the fixed 8-color palette for collaborative cursors.
// Each connected user gets a color assigned round-robin based on how
// many clients are already in the room.
var cursorColors = []string{
	"#F87171", // red
	"#FB923C", // orange
	"#FBBF24", // amber
	"#34D399", // emerald
	"#38BDF8", // sky
	"#818CF8", // indigo
	"#E879F9", // fuchsia
	"#A3E635", // lime
}

type DocumentService struct {
	documentRepo *repositories.DocumentRepository
	cardRepo     *repositories.CardRepository
	columnRepo   *repositories.ColumnRepository
	boardService *BoardService
	cfg          *config.Config
}

func NewDocumentService(
	documentRepo *repositories.DocumentRepository,
	cardRepo     *repositories.CardRepository,
	columnRepo   *repositories.ColumnRepository,
	boardService *BoardService,
	cfg          *config.Config,
) *DocumentService {
	return &DocumentService{
		documentRepo: documentRepo,
		cardRepo:     cardRepo,
		columnRepo:   columnRepo,
		boardService: boardService,
		cfg:          cfg,
	}
}

// IssueToken generates a short-lived document JWT scoped to a single document.
// Steps:
// 1. Look up the document
// 2. Walk card -> column -> board to verify the user has board access
// 3. Assign a cursor color round-robin from the palette
// 4. Generate and return the document token
// connectedCount is provided by the hub — 0 is fine before WebSocket is wired.
func (s *DocumentService) IssueToken(ctx context.Context, documentID, userID, memberRole string, connectedCount int) (*models.DocumentTokenResponse, error) {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// walk card -> column -> board to verify access
	card, err := s.cardRepo.FindByID(ctx, doc.CardID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if err := s.boardService.checkAccess(ctx, card.BoardID, userID, memberRole); err != nil {
		return nil, err
	}

	// assign cursor color round-robin
	color := cursorColors[connectedCount%len(cursorColors)]

	token, err := utils.GenerateDocumentToken(
		userID,
		documentID,
		color,
		s.cfg.JWTDocumentSecret,
		s.cfg.JWTDocumentExpiry,
	)
	if err != nil {
		return nil, err
	}

	return &models.DocumentTokenResponse{
		Token:      token,
		DocumentID: documentID,
		Color:      color,
		ExpiresIn:  3600,
	}, nil
}

// GetSnapshot returns the current document state for a client to bootstrap from.
// The snapshot is base64-encoded Yjs binary state.
// Clock tells the client which updates it still needs on top of the snapshot.
// Steps:
// 1. Look up the document and verify board access
// 2. Return snapshot (may be empty for new documents) + clock
func (s *DocumentService) GetSnapshot(ctx context.Context, documentID, userID, memberRole string) (*models.DocumentSnapshotResponse, error) {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// verify access via card -> board chain
	card, err := s.cardRepo.FindByID(ctx, doc.CardID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if err := s.boardService.checkAccess(ctx, card.BoardID, userID, memberRole); err != nil {
		return nil, err
	}

	snapshot, clock, err := s.documentRepo.GetSnapshot(ctx, documentID)
	if err != nil {
		return nil, err
	}

	// encode snapshot as base64 — empty string for new documents with no snapshot
	var snapshotB64 string
	if len(snapshot) > 0 {
		snapshotB64 = base64.StdEncoding.EncodeToString(snapshot)
	}

	return &models.DocumentSnapshotResponse{
		DocumentID: documentID,
		Snapshot:   snapshotB64,
		Clock:      clock,
	}, nil
}

// PersistUpdate is called by the WebSocket hub — not an HTTP endpoint.
// It atomically increments the document clock and appends the update.
// Returns the new clock so the hub can decide when to trigger compaction.
// CRITICAL: the hub must only broadcast AFTER this returns successfully.
func (s *DocumentService) PersistUpdate(ctx context.Context, documentID string, updateData []byte) (int, error) {
	return s.documentRepo.IncrementClock(ctx, documentID, updateData)
}

// GetUpdatesSinceClock returns all updates the client missed since its
// last known clock. Called during WebSocket reconnect sync.
func (s *DocumentService) GetUpdatesSinceClock(ctx context.Context, documentID string, clock int) ([]models.DocumentUpdate, error) {
	return s.documentRepo.GetUpdatesSinceClock(ctx, documentID, clock)
}

// CompactSnapshot merges all persisted updates into a single snapshot binary
// and deletes the individual update rows.
// Called by the hub after every 100 updates (new_clock - snapshot_clock >= 100).
// Safe to run concurrently with live edits — only compacts up to current clock.
func (s *DocumentService) CompactSnapshot(ctx context.Context, documentID string, mergedSnapshot []byte, clock int) error {
	return s.documentRepo.UpdateSnapshot(ctx, documentID, mergedSnapshot, clock)
}