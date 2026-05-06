package repo

import (
	"context"
	"database/sql"

	"coworking/internal/models"
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
