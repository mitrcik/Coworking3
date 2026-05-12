package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"coworking/internal/models"

	"github.com/lib/pq"
)

var (
	ErrWorkspaceNotFound    = errors.New("workspace not found")
	ErrWorkspaceHasBookings = errors.New("workspace has future bookings")
	ErrPositionTaken        = errors.New("position already taken by another workspace")
	ErrPositionOutOfGrid    = errors.New("position outside coworking grid")
	ErrWorkspaceNameTaken   = errors.New("workspace name already used in this coworking")
)

type WorkspaceRepo struct {
	DB *sql.DB
}

func NewWorkspaceRepo(db *sql.DB) *WorkspaceRepo { return &WorkspaceRepo{DB: db} }

// listQuery selects workspace rows; appended WHERE/ORDER BY by callers.
const listQuery = `
    SELECT workspace_id, coworking_id, name, type, zone, is_available,
           position_x, position_y, created_at
    FROM workspaces
`

// List returns all workspaces ordered by coworking + position.
func (r *WorkspaceRepo) List(ctx context.Context) ([]models.Workspace, error) {
	rows, err := r.DB.QueryContext(ctx, listQuery+`
        ORDER BY coworking_id, position_y, position_x, name
    `)
	if err != nil {
		return nil, err
	}
	return scanWorkspaces(rows)
}

// ListByCoworking returns workspaces of a given coworking, ordered by position.
func (r *WorkspaceRepo) ListByCoworking(ctx context.Context, coworkingID string) ([]models.Workspace, error) {
	rows, err := r.DB.QueryContext(ctx, listQuery+`
        WHERE coworking_id = $1
        ORDER BY position_y, position_x, name
    `, coworkingID)
	if err != nil {
		return nil, err
	}
	return scanWorkspaces(rows)
}

func scanWorkspaces(rows *sql.Rows) ([]models.Workspace, error) {
	defer rows.Close()
	var out []models.Workspace
	for rows.Next() {
		var w models.Workspace
		var t string
		if err := rows.Scan(&w.ID, &w.CoworkingID, &w.Name, &t, &w.Zone, &w.IsAvailable,
			&w.PositionX, &w.PositionY, &w.CreatedAt); err != nil {
			return nil, err
		}
		w.Type = models.WorkspaceType(t)
		out = append(out, w)
	}
	return out, rows.Err()
}

// FindByID returns a workspace or ErrWorkspaceNotFound.
func (r *WorkspaceRepo) FindByID(ctx context.Context, id string) (*models.Workspace, error) {
	row := r.DB.QueryRowContext(ctx, listQuery+` WHERE workspace_id = $1`, id)
	var w models.Workspace
	var t string
	if err := row.Scan(&w.ID, &w.CoworkingID, &w.Name, &t, &w.Zone, &w.IsAvailable,
		&w.PositionX, &w.PositionY, &w.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkspaceNotFound
		}
		return nil, err
	}
	w.Type = models.WorkspaceType(t)
	return &w, nil
}

// Create inserts a new workspace and returns its ID. Returns:
//   - ErrPositionTaken     if (coworking_id, position_x, position_y) collides;
//   - ErrWorkspaceNameTaken if (coworking_id, name) collides.
func (r *WorkspaceRepo) Create(ctx context.Context, coworkingID, name, zone string, wtype models.WorkspaceType, isAvailable bool, x, y int) (string, error) {
	const q = `
        INSERT INTO workspaces (coworking_id, name, type, zone, is_available, position_x, position_y)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING workspace_id
    `
	var id string
	if err := r.DB.QueryRowContext(ctx, q, coworkingID, name, string(wtype), zone, isAvailable, x, y).Scan(&id); err != nil {
		return "", classifyWorkspaceErr(err)
	}
	return id, nil
}

// Update modifies workspace details (including coordinates).
func (r *WorkspaceRepo) Update(ctx context.Context, id, name, zone string, wtype models.WorkspaceType, isAvailable bool, x, y int) error {
	res, err := r.DB.ExecContext(ctx, `
        UPDATE workspaces
           SET name = $2, type = $3, zone = $4, is_available = $5,
               position_x = $6, position_y = $7
         WHERE workspace_id = $1
    `, id, name, string(wtype), zone, isAvailable, x, y)
	if err != nil {
		return classifyWorkspaceErr(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrWorkspaceNotFound
	}
	return nil
}

// classifyWorkspaceErr maps Postgres unique violations to typed errors based
// on the constraint name reported by libpq.
func classifyWorkspaceErr(err error) error {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) || pqErr.Code != "23505" {
		return err
	}
	switch pqErr.Constraint {
	case "workspaces_position_per_coworking":
		return ErrPositionTaken
	case "workspaces_name_per_coworking":
		return ErrWorkspaceNameTaken
	default:
		return err
	}
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

// PositionTakenIn checks whether (x,y) is already occupied by another
// workspace within the given coworking. Pass excludeID to skip a particular
// workspace (used by update flows).
func (r *WorkspaceRepo) PositionTakenIn(ctx context.Context, coworkingID string, x, y int, excludeID string) (bool, error) {
	q := `
        SELECT EXISTS (
            SELECT 1 FROM workspaces
            WHERE coworking_id = $1 AND position_x = $2 AND position_y = $3
    `
	args := []any{coworkingID, x, y}
	if excludeID != "" {
		q += " AND workspace_id <> $4"
		args = append(args, excludeID)
	}
	q += ")"
	var exists bool
	if err := r.DB.QueryRowContext(ctx, q, args...).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
