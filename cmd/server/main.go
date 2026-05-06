package main

import (
	"log"
	"net/http"
	"os"

	"coworking/internal/auth"
	"coworking/internal/db"
	"coworking/internal/handlers"
	"coworking/internal/repo"
)

func main() {
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	conn, err := db.Open()
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer conn.Close()

	app := &handlers.App{
		Workspaces: repo.NewWorkspaceRepo(conn),
		Users:      repo.NewUserRepo(conn),
		Bookings:   repo.NewBookingRepo(conn),
		Settings:   repo.NewSettingsRepo(conn),
		Sessions:   auth.NewManager(os.Getenv("SESSION_SECRET")),
	}

	mux := http.NewServeMux()
	app.Register(mux)

	log.Printf("server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
