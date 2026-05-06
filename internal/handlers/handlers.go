package handlers

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"

	"coworking/internal/auth"
	"coworking/internal/models"
	"coworking/internal/repo"
)

// App carries shared dependencies (database repositories, sessions) for handlers.
type App struct {
	Workspaces *repo.WorkspaceRepo
	Users      *repo.UserRepo
	Sessions   *auth.Manager
}

var pageTemplates = map[string]*template.Template{}

func init() {
	layoutPath := filepath.Join("web", "templates", "layout.html")
	pages := []string{
		"home.html",
		"login.html",
		"register.html",
		"scheme.html",
		"bookings.html",
		"admin.html",
	}
	for _, p := range pages {
		pagePath := filepath.Join("web", "templates", p)
		t, err := template.ParseFiles(layoutPath, pagePath)
		if err != nil {
			log.Printf("template parse error for %s: %v", p, err)
			continue
		}
		pageTemplates[p] = t
	}
}

// Register registers all HTTP handlers on the given mux.
func (a *App) Register(mux *http.ServeMux) {
	staticDir := http.Dir(filepath.Join("web", "static"))
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(staticDir)))

	mux.HandleFunc("/", a.withUser(a.homeHandler))
	mux.HandleFunc("/login", a.loginHandler)
	mux.HandleFunc("/register", a.registerHandler)
	mux.HandleFunc("/logout", a.logoutHandler)
	mux.HandleFunc("/scheme", a.requireUser(a.schemeHandler))
	mux.HandleFunc("/bookings", a.requireUser(a.bookingsHandler))
	mux.HandleFunc("/admin", a.requireAdmin(a.adminHandler))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

type pageData struct {
	Title      string
	ActiveTab  string
	IsAdmin    bool
	IsLoggedIn bool
	UserName   string
	Flash      string
	Data       map[string]any
}

func render(w http.ResponseWriter, name string, data pageData) {
	t, ok := pageTemplates[name]
	if !ok {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("template execute error (%s): %v", name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// pageDataFor builds a pageData with auth fields populated from the request.
func pageDataFor(r *http.Request, title, tab string) pageData {
	pd := pageData{Title: title, ActiveTab: tab}
	if u, ok := auth.UserFrom(r.Context()); ok {
		pd.IsLoggedIn = true
		pd.UserName = u.FullName
		pd.IsAdmin = u.Role == models.RoleAdmin
	}
	return pd
}

func (a *App) homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	pd := pageDataFor(r, "Главная", "home")
	greeting := "Добро пожаловать в коворкинг!"
	if u, ok := auth.UserFrom(r.Context()); ok {
		greeting = "Здравствуйте, " + u.FullName + "!"
	}
	pd.Data = map[string]any{"Greeting": greeting}
	render(w, "home.html", pd)
}

// schemeWorkspace adapts a DB workspace to the template view model.
type schemeWorkspace struct {
	ID        string
	Name      string
	Type      models.WorkspaceType
	Zone      string
	IsFree    bool
	Available bool
	X         int
	Y         int
}

func (a *App) schemeHandler(w http.ResponseWriter, r *http.Request) {
	workspaces, err := a.Workspaces.List(r.Context())
	if err != nil {
		log.Printf("list workspaces: %v", err)
		http.Error(w, "Не удалось загрузить схему", http.StatusInternalServerError)
		return
	}
	views := make([]schemeWorkspace, 0, len(workspaces))
	for _, ws := range workspaces {
		views = append(views, schemeWorkspace{
			ID:        ws.ID,
			Name:      ws.Name,
			Type:      ws.Type,
			Zone:      ws.Zone,
			IsFree:    ws.IsAvailable,
			Available: ws.IsAvailable,
			X:         ws.PositionX,
			Y:         ws.PositionY,
		})
	}
	pd := pageDataFor(r, "Схема коворкинга", "scheme")
	pd.Data = map[string]any{
		"Workspaces": views,
		"Date":       "2025-12-01",
		"Start":      "09:00",
		"End":        "12:00",
	}
	render(w, "scheme.html", pd)
}

type mockBooking struct {
	ID        int
	Workspace string
	Type      string
	Date      string
	Start     string
	End       string
	Status    string
}

func mockBookings() []mockBooking {
	return []mockBooking{
		{ID: 1, Workspace: "A1", Type: "DESK", Date: "2025-12-01", Start: "09:00", End: "12:00", Status: "CONFIRMED"},
		{ID: 2, Workspace: "M1", Type: "MEETING_ROOM", Date: "2025-12-02", Start: "14:00", End: "16:00", Status: "CONFIRMED"},
		{ID: 3, Workspace: "B1", Type: "DESK", Date: "2025-11-15", Start: "10:00", End: "13:00", Status: "COMPLETED"},
		{ID: 4, Workspace: "L1", Type: "LOUNGE", Date: "2025-11-20", Start: "15:00", End: "17:00", Status: "CANCELLED_BY_USER"},
	}
}

func (a *App) bookingsHandler(w http.ResponseWriter, r *http.Request) {
	pd := pageDataFor(r, "Мои бронирования", "bookings")
	pd.Data = map[string]any{"Bookings": mockBookings()}
	render(w, "bookings.html", pd)
}

func (a *App) adminHandler(w http.ResponseWriter, r *http.Request) {
	workspaces, err := a.Workspaces.List(r.Context())
	if err != nil {
		log.Printf("admin list workspaces: %v", err)
		http.Error(w, "Не удалось загрузить данные админки", http.StatusInternalServerError)
		return
	}
	views := make([]schemeWorkspace, 0, len(workspaces))
	for _, ws := range workspaces {
		views = append(views, schemeWorkspace{
			ID:        ws.ID,
			Name:      ws.Name,
			Type:      ws.Type,
			Zone:      ws.Zone,
			IsFree:    ws.IsAvailable,
			Available: ws.IsAvailable,
			X:         ws.PositionX,
			Y:         ws.PositionY,
		})
	}
	pd := pageDataFor(r, "Админ-панель", "admin")
	pd.Data = map[string]any{
		"Workspaces":     views,
		"Bookings":       mockBookings(),
		"MaxActiveLimit": 3,
	}
	render(w, "admin.html", pd)
}
