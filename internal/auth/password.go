package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword returns a bcrypt hash compatible with the seed admin record
// (created in PostgreSQL via pgcrypto's crypt(... 'bf')).
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// VerifyPassword returns nil if the plain text matches the stored hash.
func VerifyPassword(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
