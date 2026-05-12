package repo

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
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

// BusyDetail describes a workspace + the booking that occupies it.
type BusyDetail struct {
	WorkspaceID string
	StartTime   time.Time
	EndTime     time.Time
}

// BusyDetailsForWorkspaces returns the first overlapping confirmed booking per
// workspace within [start, end). When multiple bookings overlap the interval
// we keep the one ending earliest, which is the most useful label ("свободно
// после X").
func (r *BookingRepo) BusyDetailsForWorkspaces(ctx context.Context, start, end time.Time) ([]BusyDetail, error) {
	const q = `
        SELECT DISTINCT ON (workspace_id) workspace_id, start_time, end_time
        FROM bookings
        WHERE status = 'CONFIRMED'
          AND start_time < $2
          AND end_time   > $1
        ORDER BY workspace_id, end_time ASC
    `
	rows, err := r.DB.QueryContext(ctx, q, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BusyDetail
	for rows.Next() {
		var b BusyDetail
		if err := rows.Scan(&b.WorkspaceID, &b.StartTime, &b.EndTime); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
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

// CooldownUntil returns the time at which the user-cancellation cool-down
// (3+ cancellations within the last 24 h → 12 h block) expires.
//
// If the user is not currently blocked, a zero time.Time is returned.
// `now` is the reference moment for the lookback window and the threshold
// is computed from the moment of the user's third most recent cancellation
// in the last 24 h (plus 12 h).
func (r *BookingRepo) CooldownUntil(ctx context.Context, userID string, now time.Time) (time.Time, error) {
	since := now.Add(-24 * time.Hour)
	const q = `
        WITH recent AS (
            SELECT cancelled_at
            FROM bookings
            WHERE user_id = $1
              AND status = 'CANCELLED_BY_USER'
              AND cancelled_at IS NOT NULL
              AND cancelled_at >= $2
            ORDER BY cancelled_at DESC
            LIMIT 3
        )
        SELECT MIN(cancelled_at), COUNT(*) FROM recent
    `
	var oldest sql.NullTime
	var n int
	if err := r.DB.QueryRowContext(ctx, q, userID, since).Scan(&oldest, &n); err != nil {
		return time.Time{}, err
	}
	if n < 3 || !oldest.Valid {
		return time.Time{}, nil
	}
	until := oldest.Time.Add(12 * time.Hour)
	if !until.After(now) {
		return time.Time{}, nil
	}
	return until, nil
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

// BookingView is a row joined with workspace and user info for display.
type BookingView struct {
	models.Booking
	WorkspaceName string
	WorkspaceType models.WorkspaceType
	Zone          string
	UserEmail     string
	UserFullName  string
}

const baseSelect = `
    SELECT b.booking_id, b.user_id, b.workspace_id, b.start_time, b.end_time, b.status,
           b.created_at, b.cancelled_at,
           w.name, w.type, w.zone,
           u.email, u.full_name
    FROM bookings b
    JOIN workspaces w ON w.workspace_id = b.workspace_id
    JOIN users u      ON u.user_id      = b.user_id
`

// ListByUser returns bookings for the given user, optionally filtered by status.
func (r *BookingRepo) ListByUser(ctx context.Context, userID string, statuses []models.BookingStatus) ([]BookingView, error) {
	q := baseSelect + ` WHERE b.user_id = $1`
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

// ListByWorkspace returns CONFIRMED bookings linked to a workspace.
// The optional [from, to) interval narrows the result; pass zero values to
// skip a bound.
func (r *BookingRepo) ListByWorkspace(ctx context.Context, workspaceID string, from, to time.Time) ([]BookingView, error) {
	q := baseSelect + ` WHERE b.workspace_id = $1 AND b.status = 'CONFIRMED'`
	args := []any{workspaceID}
	if !from.IsZero() {
		args = append(args, from)
		q += " AND b.end_time > $" + strconv.Itoa(len(args))
	}
	if !to.IsZero() {
		args = append(args, to)
		q += " AND b.start_time < $" + strconv.Itoa(len(args))
	}
	q += " ORDER BY b.start_time ASC"
	return r.queryViews(ctx, q, args...)
}

// CountActiveByUserExcluding counts CONFIRMED bookings that still belong to
// the user and overlap [now, +∞). Same as CountActiveByUser but with the
// option of excluding a particular booking_id (useful for "edit" flows).
func (r *BookingRepo) CountActiveByUserExcluding(ctx context.Context, userID string, now time.Time, excludeBookingID string) (int, error) {
	q := `
        SELECT COUNT(*) FROM bookings
        WHERE user_id = $1 AND status = 'CONFIRMED' AND end_time > $2
    `
	args := []any{userID, now}
	if excludeBookingID != "" {
		q += " AND booking_id <> $3"
		args = append(args, excludeBookingID)
	}
	var n int
	if err := r.DB.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// AdminBookingFilter narrows the admin booking list.
type AdminBookingFilter struct {
	Status      string // CONFIRMED / COMPLETED / CANCELLED_BY_USER / CANCELLED_BY_ADMIN
	UserEmail   string // substring match
	WorkspaceID string
	From        *time.Time // start_time >= From
	To          *time.Time // start_time <  To
}

// ListAll returns every booking with optional filters (for admin views).
func (r *BookingRepo) ListAll(ctx context.Context, f AdminBookingFilter) ([]BookingView, error) {
	q := baseSelect + " WHERE 1=1"
	args := []any{}
	add := func(clause string, val any) {
		args = append(args, val)
		q += " AND " + clause + " $" + strconv.Itoa(len(args))
	}
	if f.Status != "" {
		add("b.status =", f.Status)
	}
	if f.WorkspaceID != "" {
		add("b.workspace_id =", f.WorkspaceID)
	}
	if f.UserEmail != "" {
		add("u.email ILIKE", "%"+f.UserEmail+"%")
	}
	if f.From != nil {
		add("b.start_time >=", *f.From)
	}
	if f.To != nil {
		add("b.start_time <", *f.To)
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
		v, err := scanBookingView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type rowScanner interface{ Scan(...any) error }

func scanBookingView(s rowScanner) (BookingView, error) {
	var v BookingView
	var status, wtype string
	var cancelled sql.NullTime
	if err := s.Scan(
		&v.ID, &v.UserID, &v.WorkspaceID, &v.StartTime, &v.EndTime, &status,
		&v.CreatedAt, &cancelled,
		&v.WorkspaceName, &wtype, &v.Zone,
		&v.UserEmail, &v.UserFullName,
	); err != nil {
		return v, err
	}
	v.Status = models.BookingStatus(status)
	v.WorkspaceType = models.WorkspaceType(wtype)
	if cancelled.Valid {
		t := cancelled.Time
		v.CancelledAt = &t
	}
	return v, nil
}

func (r *BookingRepo) FindByID(ctx context.Context, id string) (*BookingView, error) {
	row := r.DB.QueryRowContext(ctx, baseSelect+` WHERE b.booking_id = $1`, id)
	v, err := scanBookingView(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBookingNotFound
		}
		return nil, err
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
