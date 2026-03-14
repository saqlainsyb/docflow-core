package repositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saqlainsyb/docflow-core/internal/models"
)

type WorkspaceRepository struct {
	db *pgxpool.Pool
}

func NewWorkspaceRepository(db *pgxpool.Pool) *WorkspaceRepository {
	return &WorkspaceRepository{db: db}
}

// CreateWithOwner inserts a new workspace and adds the creator as 'owner'
// in workspace_members — both in a single transaction.
// Called from auth service (auto-create on register) and workspace service
// (explicit create endpoint).
func (r *WorkspaceRepository) CreateWithOwner(ctx context.Context, name, ownerID string) (*models.Workspace, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	ws := &models.Workspace{}
	err = tx.QueryRow(ctx, `
		INSERT INTO workspaces (name, owner_id)
		VALUES ($1, $2)
		RETURNING id, name, owner_id, created_at, updated_at
	`, name, ownerID).Scan(
		&ws.ID,
		&ws.Name,
		&ws.OwnerID,
		&ws.CreatedAt,
		&ws.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO workspace_members (workspace_id, user_id, role)
		VALUES ($1, $2, 'owner')
	`, ws.ID, ownerID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return ws, nil
}

// FindByID looks up a workspace by UUID.
// Returns ErrNotFound if no workspace exists with that ID.
func (r *WorkspaceRepository) FindByID(ctx context.Context, id string) (*models.Workspace, error) {
	ws := &models.Workspace{}
	err := r.db.QueryRow(ctx, `
		SELECT id, name, owner_id, created_at, updated_at
		FROM workspaces
		WHERE id = $1
	`, id).Scan(
		&ws.ID,
		&ws.Name,
		&ws.OwnerID,
		&ws.CreatedAt,
		&ws.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return ws, nil
}

// FindByUserID returns all workspaces the user belongs to (any role).
// Ordered by creation time so the list is stable.
func (r *WorkspaceRepository) FindByUserID(ctx context.Context, userID string) ([]models.WorkspaceResponse, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			w.id,
			w.name,
			w.owner_id,
			w.created_at,
			(SELECT COUNT(*) FROM workspace_members wm2 WHERE wm2.workspace_id = w.id) AS member_count
		FROM workspaces w
		JOIN workspace_members wm ON wm.workspace_id = w.id
		WHERE wm.user_id = $1
		ORDER BY w.created_at ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []models.WorkspaceResponse
	for rows.Next() {
		var ws models.WorkspaceResponse
		if err := rows.Scan(&ws.ID, &ws.Name, &ws.OwnerID, &ws.CreatedAt, &ws.MemberCount); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, ws)
	}

	return workspaces, rows.Err()
}

// Update renames a workspace. Only the name can change in V1.
// Returns the updated workspace.
func (r *WorkspaceRepository) Update(ctx context.Context, id, name string) (*models.Workspace, error) {
	ws := &models.Workspace{}
	err := r.db.QueryRow(ctx, `
		UPDATE workspaces
		SET name = $1, updated_at = NOW()
		WHERE id = $2
		RETURNING id, name, owner_id, created_at, updated_at
	`, name, id).Scan(
		&ws.ID,
		&ws.Name,
		&ws.OwnerID,
		&ws.CreatedAt,
		&ws.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return ws, nil
}

// Delete removes the workspace. Cascading FK constraints in the schema
// automatically delete boards, columns, cards, documents, and all member rows.
func (r *WorkspaceRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// GetMember returns a single membership row for a (workspace, user) pair.
// Used by the workspace middleware to check membership and inject the role.
// Returns ErrNotFound if the user is not a member of the workspace.
func (r *WorkspaceRepository) GetMember(ctx context.Context, workspaceID, userID string) (*models.WorkspaceMember, error) {
	m := &models.WorkspaceMember{}
	err := r.db.QueryRow(ctx, `
		SELECT workspace_id, user_id, role, joined_at
		FROM workspace_members
		WHERE workspace_id = $1 AND user_id = $2
	`, workspaceID, userID).Scan(
		&m.WorkspaceID,
		&m.UserID,
		&m.Role,
		&m.JoinedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return m, nil
}

// AddMember inserts a new row into workspace_members with the given role.
// The caller is responsible for checking the user exists and is not already a member.
func (r *WorkspaceRepository) AddMember(ctx context.Context, workspaceID, userID, role string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO workspace_members (workspace_id, user_id, role)
		VALUES ($1, $2, $3)
	`, workspaceID, userID, role)

	return err
}

// UpdateMemberRole changes the role of an existing member.
func (r *WorkspaceRepository) UpdateMemberRole(ctx context.Context, workspaceID, userID, role string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE workspace_members
		SET role = $1
		WHERE workspace_id = $2 AND user_id = $3
	`, role, workspaceID, userID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// RemoveMember deletes a membership row.
// The caller is responsible for enforcing role-based permission rules before calling this.
func (r *WorkspaceRepository) RemoveMember(ctx context.Context, workspaceID, userID string) error {
	result, err := r.db.Exec(ctx, `
		DELETE FROM workspace_members
		WHERE workspace_id = $1 AND user_id = $2
	`, workspaceID, userID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// ListMembers returns all members of a workspace with their user details.
// The JOIN with users means the service gets back the full MemberResponse DTO
// without needing a second query.
func (r *WorkspaceRepository) ListMembers(ctx context.Context, workspaceID string) ([]models.MemberResponse, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			u.id,
			u.name,
			u.email,
			u.avatar_url,
			wm.role,
			wm.joined_at
		FROM workspace_members wm
		JOIN users u ON u.id = wm.user_id
		WHERE wm.workspace_id = $1
		ORDER BY wm.joined_at ASC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.MemberResponse
	for rows.Next() {
		var m models.MemberResponse
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &m.AvatarURL, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}

	return members, rows.Err()
}
