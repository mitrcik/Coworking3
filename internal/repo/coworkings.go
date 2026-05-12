package repo

import (
	"context"
	"database/sql"
	"errors"

	"coworking/internal/models"
)

var (
	ErrCoworkingNotFound         = errors.New("coworking not found")
	ErrCoworkingHasWorkspacesOut = errors.New("coworking has workspaces outside new grid")
	ErrCoworkingHasWorkspaces    = errors.New("coworking has workspaces")
	ErrCoworkingNameTaken        = errors.New("coworking name already used")
)

// MaxGridDimension is the upper bound for grid_cols and grid_rows. The DB has
// a matching CHECK constraint; we duplicate it here so handlers can reject
// bad input with a friendly error before the round-trip.
const MaxGridDimension = 20

type CoworkingRepo struct {
	DB *sql.DB
}

func NewCoworkingRepo(db *sql.DB) *CoworkingRepo { return &CoworkingRepo{DB: db} }

// List returns all coworkings ordered by name.
func (r *CoworkingRepo) List(ctx context.Context) ([]models.Coworking, error) {
	const q = `
        SELECT coworking_id, name, grid_cols, grid_rows, created_at
        FROM coworkings
        ORDER BY name ASC
    `
	rows, err := r.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Coworking
	for rows.Next() {
		var c models.Coworking
		if err := rows.Scan(&c.ID, &c.Name, &c.GridCols, &c.GridRows, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// FindByID returns a coworking or ErrCoworkingNotFound.
func (r *CoworkingRepo) FindByID(ctx context.Context, id string) (*models.Coworking, error) {
	const q = `
        SELECT coworking_id, name, grid_cols, grid_rows, created_at
        FROM coworkings WHERE coworking_id = $1
    `
	row := r.DB.QueryRowContext(ctx, q, id)
	var c models.Coworking
	if err := row.Scan(&c.ID, &c.Name, &c.GridCols, &c.GridRows, &c.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCoworkingNotFound
		}
		return nil, err
	}
	return &c, nil
}

// Create inserts a new coworking.
func (r *CoworkingRepo) Create(ctx context.Context, name string, cols, rowsN int) (string, error) {
	const q = `
        INSERT INTO coworkings (name, grid_cols, grid_rows)
        VALUES ($1, $2, $3)
        RETURNING coworking_id
    `
	var id string
	if err := r.DB.QueryRowContext(ctx, q, name, cols, rowsN).Scan(&id); err != nil {
		if isUniqueViolation(err) {
			return "", ErrCoworkingNameTaken
		}
		return "", err
	}
	return id, nil
}

// Update changes name and grid. Returns ErrCoworkingHasWorkspacesOut if any
// existing workspace would fall outside the new grid.
func (r *CoworkingRepo) Update(ctx context.Context, id, name string, cols, rowsN int) error {
	var outside int
	if err := r.DB.QueryRowContext(ctx, `
        SELECT COUNT(*) FROM workspaces
        WHERE coworking_id = $1 AND (position_x > $2 OR position_y > $3)
    `, id, cols, rowsN).Scan(&outside); err != nil {
		return err
	}
	if outside > 0 {
		return ErrCoworkingHasWorkspacesOut
	}
	res, err := r.DB.ExecContext(ctx, `
        UPDATE coworkings SET name = $2, grid_cols = $3, grid_rows = $4
        WHERE coworking_id = $1
    `, id, name, cols, rowsN)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrCoworkingNameTaken
		}
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCoworkingNotFound
	}
	return nil
}

// Delete removes a coworking. Returns ErrCoworkingHasWorkspaces if any
// workspace is still attached.
func (r *CoworkingRepo) Delete(ctx context.Context, id string) error {
	var n int
	if err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workspaces WHERE coworking_id = $1`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrCoworkingHasWorkspaces
	}
	res, err := r.DB.ExecContext(ctx, `DELETE FROM coworkings WHERE coworking_id = $1`, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrCoworkingNotFound
	}
	return nil
}
