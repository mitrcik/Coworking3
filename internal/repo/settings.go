package repo

import (
	"context"
	"database/sql"
	"errors"

	"coworking/internal/models"
)

var ErrSettingsNotFound = errors.New("booking settings not found")

type SettingsRepo struct {
	DB *sql.DB
}

func NewSettingsRepo(db *sql.DB) *SettingsRepo { return &SettingsRepo{DB: db} }

// Get returns the singleton booking settings row.
func (r *SettingsRepo) Get(ctx context.Context) (*models.BookingSettings, error) {
	const q = `
        SELECT settings_id, max_active_bookings_per_user, cancellation_lead_time_hours, updated_by, updated_at
        FROM booking_settings LIMIT 1
    `
	row := r.DB.QueryRowContext(ctx, q)
	var s models.BookingSettings
	var updatedBy sql.NullString
	if err := row.Scan(&s.ID, &s.MaxActiveBookingsPerUser, &s.CancellationLeadTimeHours, &updatedBy, &s.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSettingsNotFound
		}
		return nil, err
	}
	if updatedBy.Valid {
		v := updatedBy.String
		s.UpdatedBy = &v
	}
	return &s, nil
}

// Update modifies the singleton booking settings.
func (r *SettingsRepo) Update(ctx context.Context, maxActive, leadHours int, updatedBy string) error {
	res, err := r.DB.ExecContext(ctx, `
        UPDATE booking_settings
           SET max_active_bookings_per_user = $1,
               cancellation_lead_time_hours = $2,
               updated_by = $3,
               updated_at = NOW()
    `, maxActive, leadHours, updatedBy)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSettingsNotFound
	}
	return nil
}
