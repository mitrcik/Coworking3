package repo

import (
	"errors"

	"github.com/lib/pq"
)

// pqArray wraps a string slice for use with PostgreSQL `= ANY($n)` clauses.
func pqArray(vals []string) any { return pq.Array(vals) }

// isUniqueViolation reports whether the error is a Postgres unique constraint
// violation (SQLSTATE 23505). Used by repos to translate FK/uniqueness
// collisions into typed errors.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505"
	}
	return false
}
