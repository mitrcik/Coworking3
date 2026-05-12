package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"coworking/internal/models"
)

const sessionCookie = "coworking_session"

type session struct {
	UserID    string
	ExpiresAt time.Time
}

// Manager is a simple in-memory session manager backed by a signed cookie.
// In a real product you'd persist sessions in Redis or a sessions table; for a
// learning prototype an in-memory map is enough and keeps the code minimal.
type Manager struct {
	secret []byte
	mu     sync.RWMutex
	store  map[string]session
}

func NewManager(secret string) *Manager {
	if secret == "" {
		secret = "dev-secret-please-change-in-production"
	}
	return &Manager{
		secret: []byte(secret),
		store:  make(map[string]session),
	}
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (m *Manager) sign(token string) string {
	h := hmac.New(sha256.New, m.secret)
	_, _ = h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}

// Login creates a session for the given user and writes the cookie.
func (m *Manager) Login(w http.ResponseWriter, userID string) error {
	token, err := newToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(24 * time.Hour * 7)

	m.mu.Lock()
	m.store[token] = session{UserID: userID, ExpiresAt: expires}
	m.mu.Unlock()

	signed := token + "." + m.sign(token)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    signed,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// Logout invalidates the current session (if any) and clears the cookie.
func (m *Manager) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		token, _, ok := splitSigned(c.Value)
		if ok {
			m.mu.Lock()
			delete(m.store, token)
			m.mu.Unlock()
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// CurrentUserID extracts and validates the session and returns the userID.
func (m *Manager) CurrentUserID(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}
	token, sig, ok := splitSigned(c.Value)
	if !ok {
		return "", false
	}
	if !hmac.Equal([]byte(sig), []byte(m.sign(token))) {
		return "", false
	}
	m.mu.RLock()
	s, ok := m.store[token]
	m.mu.RUnlock()
	if !ok || time.Now().After(s.ExpiresAt) {
		return "", false
	}
	return s.UserID, true
}

func splitSigned(raw string) (token, sig string, ok bool) {
	idx := strings.LastIndex(raw, ".")
	if idx < 1 || idx == len(raw)-1 {
		return "", "", false
	}
	return raw[:idx], raw[idx+1:], true
}

// User context helpers --------------------------------------------------------

type ctxKey int

const userKey ctxKey = 0

func WithUser(ctx context.Context, u *models.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

func UserFrom(ctx context.Context) (*models.User, bool) {
	u, ok := ctx.Value(userKey).(*models.User)
	return u, ok && u != nil
}

var ErrUnauthorized = errors.New("unauthorized")
