package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"coworking/internal/models"
)

var ErrBookingNotFound = errors.New("booking not found")

type BookingRepo struct {
	DB *sql.DB
}

func NewBookingRepo(db *sql.DB) *BookingRepo { return &BookingRepo{DB: db} }

// BusyWorkspaceIDs returns IDs of workspaces that already have a CONFIRMED
// booking overlapping the [start, end) interval. Two intervals overlap when
//   existing.start_time < end AND existing.end_time > start
func (r *BookingRepo) BusyWorkspaceIDs(ctx context.Context, start, end time.Time) (map[string]struct{}, error) {
	const q = `
        SELECT DISTINCT workspace_id
        FROM bookings
        WHERE status = 'CONFIRMED'
          AND start_time < $2
          AND end_time   > $1
    `
	rows, err := r.DB.QueryContext(ctx, q, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = struct{}{}
	}
	return ids, rows.Err()
}

// HasConflict checks if a workspace already has a confirmed overlapping booking.
func (r *BookingRepo) HasConflict(ctx context.Context, workspaceID string, start, end time.Time) (bool, error) {
	const q = `
        SELECT EXISTS(
            SELECT 1 FROM bookings
            WHERE workspace_id = $1
              AND status = 'CONFIRMED'
              AND start_time < $3
              AND end_time   > $2
        )
    `
	var exists bool
	if err := r.DB.QueryRowContext(ctx, q, workspaceID, start, end).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// HasUserConflict checks whether the same user has a confirmed overlapping
// booking on any workspace.
func (r *BookingRepo) HasUserConflict(ctx context.Context, userID string, start, end time.Time) (bool, error) {
	const q = `
        SELECT EXISTS(
            SELECT 1 FROM bookings
            WHERE user_id = $1
              AND status = 'CONFIRMED'
              AND start_time < $3
              AND end_time   > $2
        )
    `
	var exists bool
	if err := r.DB.QueryRowContext(ctx, q, userID, start, end).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// CountActiveByUser counts CONFIRMED bookings for a user that have not ended yet.
func (r *BookingRepo) CountActiveByUser(ctx context.Context, userID string, now time.Time) (int, error) {
	const q = `
        SELECT COUNT(*) FROM bookings
        WHERE user_id = $1
          AND status = 'CONFIRMED'
          AND end_time > $2
    `
	var n int
	if err := r.DB.QueryRowContext(ctx, q, userID, now).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// Create inserts a new booking and returns its ID.
func (r *BookingRepo) Create(ctx context.Context, userID, workspaceID string, start, end time.Time) (string, error) {
	const q = `
        INSERT INTO bookings (user_id, workspace_id, start_time, end_time, status)
        VALUES ($1, $2, $3, $4, 'CONFIRMED')
        RETURNING booking_id
    `
	var id string
	if err := r.DB.QueryRowContext(ctx, q, userID, workspaceID, start, end).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

// BookingView is a row joined with workspace name/type for display.
type BookingView struct {
	models.Booking
	WorkspaceName string
	WorkspaceType models.WorkspaceType
	Zone          string
}

// ListByUser returns bookings for the given user, optionally filtered by status.
func (r *BookingRepo) ListByUser(ctx context.Context, userID string, statuses []models.BookingStatus) ([]BookingView, error) {
	q := `
        SELECT b.booking_id, b.user_id, b.workspace_id, b.start_time, b.end_time, b.status,
               b.created_at, b.cancelled_at,
               w.name, w.type, w.zone
        FROM bookings b
        JOIN workspaces w ON w.workspace_id = b.workspace_id
        WHERE b.user_id = $1
    `
	args := []any{userID}
	if len(statuses) > 0 {
		q += " AND b.status = ANY($2)"
		statusStrs := make([]string, len(statuses))
		for i, s := range statuses {
			statusStrs[i] = string(s)
		}
		args = append(args, pqArray(statusStrs))
	}
	q += " ORDER BY b.start_time DESC"
	return r.queryViews(ctx, q, args...)
}

// ListAll returns every booking (used by admin views).
func (r *BookingRepo) ListAll(ctx context.Context, statuses []models.BookingStatus) ([]BookingView, error) {
	q := `
        SELECT b.booking_id, b.user_id, b.workspace_id, b.start_time, b.end_time, b.status,
               b.created_at, b.cancelled_at,
               w.name, w.type, w.zone
        FROM bookings b
        JOIN workspaces w ON w.workspace_id = b.workspace_id
    `
	args := []any{}
	if len(statuses) > 0 {
		q += " WHERE b.status = ANY($1)"
		statusStrs := make([]string, len(statuses))
		for i, s := range statuses {
			statusStrs[i] = string(s)
		}
		args = append(args, pqArray(statusStrs))
	}
	q += " ORDER BY b.start_time DESC"
	return r.queryViews(ctx, q, args...)
}

func (r *BookingRepo) queryViews(ctx context.Context, q string, args ...any) ([]BookingView, error) {
	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BookingView
	for rows.Next() {
		var v BookingView
		var status, wtype string
		var cancelled sql.NullTime
		if err := rows.Scan(
			&v.ID, &v.UserID, &v.WorkspaceID, &v.StartTime, &v.EndTime, &status,
			&v.CreatedAt, &cancelled,
			&v.WorkspaceName, &wtype, &v.Zone,
		); err != nil {
			return nil, err
		}
		v.Status = models.BookingStatus(status)
		v.WorkspaceType = models.WorkspaceType(wtype)
		if cancelled.Valid {
			t := cancelled.Time
			v.CancelledAt = &t
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *BookingRepo) FindByID(ctx context.Context, id string) (*BookingView, error) {
	const q = `
        SELECT b.booking_id, b.user_id, b.workspace_id, b.start_time, b.end_time, b.status,
               b.created_at, b.cancelled_at,
               w.name, w.type, w.zone
        FROM bookings b
        JOIN workspaces w ON w.workspace_id = b.workspace_id
        WHERE b.booking_id = $1
    `
	row := r.DB.QueryRowContext(ctx, q, id)
	var v BookingView
	var status, wtype string
	var cancelled sql.NullTime
	if err := row.Scan(
		&v.ID, &v.UserID, &v.WorkspaceID, &v.StartTime, &v.EndTime, &status,
		&v.CreatedAt, &cancelled,
		&v.WorkspaceName, &wtype, &v.Zone,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBookingNotFound
		}
		return nil, err
	}
	v.Status = models.BookingStatus(status)
	v.WorkspaceType = models.WorkspaceType(wtype)
	if cancelled.Valid {
		t := cancelled.Time
		v.CancelledAt = &t
	}
	return &v, nil
}

func (r *BookingRepo) Cancel(ctx context.Context, id string, byAdmin bool) error {
	status := models.StatusCancelledByUser
	if byAdmin {
		status = models.StatusCancelledByAdmin
	}
	res, err := r.DB.ExecContext(ctx, `
        UPDATE bookings
           SET status = $2, cancelled_at = NOW()
         WHERE booking_id = $1 AND status = 'CONFIRMED'
    `, id, string(status))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrBookingNotFound
	}
	return nil
}

func (r *BookingRepo) UpdateStatus(ctx context.Context, id string, status models.BookingStatus) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE bookings SET status = $2 WHERE booking_id = $1`, id, string(status))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrBookingNotFound
	}
	return nil
}
