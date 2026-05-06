package repo

import (
	"context"
	"database/sql"
	"errors"

	"coworking/internal/models"
)

var ErrUserNotFound = errors.New("user not found")

type UserRepo struct {
	DB *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo { return &UserRepo{DB: db} }

func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	const q = `
        SELECT user_id, full_name, email, password_hash, role, active_booking_count, created_at
        FROM users WHERE email = $1
    `
	row := r.DB.QueryRowContext(ctx, q, email)
	var u models.User
	var role string
	if err := row.Scan(&u.ID, &u.FullName, &u.Email, &u.PasswordHash, &role, &u.ActiveBookingCount, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	u.Role = models.Role(role)
	return &u, nil
}

func (r *UserRepo) FindByID(ctx context.Context, id string) (*models.User, error) {
	const q = `
        SELECT user_id, full_name, email, password_hash, role, active_booking_count, created_at
        FROM users WHERE user_id = $1
    `
	row := r.DB.QueryRowContext(ctx, q, id)
	var u models.User
	var role string
	if err := row.Scan(&u.ID, &u.FullName, &u.Email, &u.PasswordHash, &role, &u.ActiveBookingCount, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	u.Role = models.Role(role)
	return &u, nil
}

// Create inserts a user and returns the new ID. Caller must pass an already
// hashed password.
func (r *UserRepo) Create(ctx context.Context, fullName, email, passwordHash string, role models.Role) (string, error) {
	const q = `
        INSERT INTO users (full_name, email, password_hash, role)
        VALUES ($1, $2, $3, $4)
        RETURNING user_id
    `
	var id string
	if err := r.DB.QueryRowContext(ctx, q, fullName, email, passwordHash, string(role)).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

// EmailExists checks whether an email is already registered.
func (r *UserRepo) EmailExists(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`, email).Scan(&exists)
	return exists, err
}
