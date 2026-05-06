package repo

import "github.com/lib/pq"

// pqArray wraps a string slice for use with PostgreSQL `= ANY($n)` clauses.
func pqArray(vals []string) any { return pq.Array(vals) }
