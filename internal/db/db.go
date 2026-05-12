package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// Open connects to PostgreSQL using DB_* environment variables. It retries the
// initial connection so the app can wait for the database container to come up.
func Open() (*sql.DB, error) {
	host := getenv("DB_HOST", "localhost")
	port := getenv("DB_PORT", "5432")
	user := getenv("DB_USER", "coworking")
	pass := getenv("DB_PASSWORD", "coworking")
	name := getenv("DB_NAME", "coworking")

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, pass, name,
	)

	var db *sql.DB
	var err error
	for i := 0; i < 30; i++ {
		db, err = sql.Open("postgres", dsn)
		if err == nil {
			if err = db.Ping(); err == nil {
				log.Printf("connected to postgres %s:%s/%s", host, port, name)
				db.SetMaxOpenConns(20)
				db.SetMaxIdleConns(5)
				db.SetConnMaxLifetime(time.Hour)
				return db, nil
			}
		}
		log.Printf("waiting for postgres (attempt %d): %v", i+1, err)
		time.Sleep(time.Second)
	}
	return nil, fmt.Errorf("postgres unreachable: %w", err)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
