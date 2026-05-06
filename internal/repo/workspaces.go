package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"coworking/internal/models"
)

var (
	ErrWorkspaceNotFound    = errors.New("workspace not found")
	ErrWorkspaceHasBookings = errors.New("workspace has future bookings")
)

type WorkspaceRepo struct {
	DB *sql.DB
}

func NewWorkspaceRepo(db *sql.DB) *WorkspaceRepo { return &WorkspaceRepo{DB: db} }

// List returns all workspaces ordered by position for stable scheme rendering.
func (r *WorkspaceRepo) List(ctx context.Context) ([]models.Workspace, error) {
	const q = `
        SELECT workspace_id, name, type, zone, is_available, position_x, position_y, created_at
        FROM workspaces
        ORDER BY position_y, position_x, name
    `
	rows, err := r.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Workspace
	for rows.Next() {
		var w models.Workspace
		var t string
		if err := rows.Scan(&w.ID, &w.Name, &t, &w.Zone, &w.IsAvailable, &w.PositionX, &w.PositionY, &w.CreatedAt); err != nil {
			return nil, err
		}
		w.Type = models.WorkspaceType(t)
		out = append(out, w)
	}
	return out, rows.Err()
}

// FindByID returns a workspace or ErrWorkspaceNotFound.
func (r *WorkspaceRepo) FindByID(ctx context.Context, id string) (*models.Workspace, error) {
	const q = `
        SELECT workspace_id, name, type, zone, is_available, position_x, position_y, created_at
        FROM workspaces WHERE workspace_id = $1
    `
	row := r.DB.QueryRowContext(ctx, q, id)
	var w models.Workspace
	var t string
	if err := row.Scan(&w.ID, &w.Name, &t, &w.Zone, &w.IsAvailable, &w.PositionX, &w.PositionY, &w.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkspaceNotFound
		}
		return nil, err
	}
	w.Type = models.WorkspaceType(t)
	return &w, nil
}

// Create inserts a new workspace and returns its ID.
func (r *WorkspaceRepo) Create(ctx context.Context, name, zone string, wtype models.WorkspaceType, isAvailable bool, x, y int) (string, error) {
	const q = `
        INSERT INTO workspaces (name, type, zone, is_available, position_x, position_y)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING workspace_id
    `
	var id string
	if err := r.DB.QueryRowContext(ctx, q, name, string(wtype), zone, isAvailable, x, y).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

// Update modifies workspace details.
func (r *WorkspaceRepo) Update(ctx context.Context, id, name, zone string, wtype models.WorkspaceType, isAvailable bool, x, y int) error {
	res, err := r.DB.ExecContext(ctx, `
        UPDATE workspaces
           SET name = $2, type = $3, zone = $4, is_available = $5, position_x = $6, position_y = $7
         WHERE workspace_id = $1
    `, id, name, string(wtype), zone, isAvailable, x, y)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrWorkspaceNotFound
	}
	return nil
}

// Delete removes a workspace. Returns ErrWorkspaceHasBookings if there are
// future CONFIRMED bookings linked to it.
func (r *WorkspaceRepo) Delete(ctx context.Context, id string, now time.Time) error {
	var hasFuture bool
	if err := r.DB.QueryRowContext(ctx, `
        SELECT EXISTS(
            SELECT 1 FROM bookings
            WHERE workspace_id = $1 AND status = 'CONFIRMED' AND end_time > $2
        )
    `, id, now).Scan(&hasFuture); err != nil {
		return err
	}
	if hasFuture {
		return ErrWorkspaceHasBookings
	}
	res, err := r.DB.ExecContext(ctx, `DELETE FROM workspaces WHERE workspace_id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrWorkspaceNotFound
	}
	return nil
}

// SetAvailable toggles `is_available` without deleting bookings.
func (r *WorkspaceRepo) SetAvailable(ctx context.Context, id string, available bool) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE workspaces SET is_available = $2 WHERE workspace_id = $1`, id, available)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrWorkspaceNotFound
	}
	return nil
}
