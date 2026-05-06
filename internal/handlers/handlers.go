package handlers

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"
)

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

// Register registers all HTTP handlers.
func Register(mux *http.ServeMux) {
	staticDir := http.Dir(filepath.Join("web", "static"))
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(staticDir)))

	mux.HandleFunc("/", homeHandler)
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/register", registerHandler)
	mux.HandleFunc("/scheme", schemeHandler)
	mux.HandleFunc("/bookings", bookingsHandler)
	mux.HandleFunc("/admin", adminHandler)
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

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	render(w, "home.html", pageData{
		Title:      "Главная",
		ActiveTab:  "home",
		IsLoggedIn: false,
		UserName:   "Гость",
		Data: map[string]any{
			"Greeting": "Добро пожаловать в коворкинг!",
		},
	})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	render(w, "login.html", pageData{
		Title:     "Вход",
		ActiveTab: "auth",
	})
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	render(w, "register.html", pageData{
		Title:     "Регистрация",
		ActiveTab: "auth",
	})
}

// mockWorkspace is used only in stage 1 to render the layout.
type mockWorkspace struct {
	ID        int
	Name      string
	Type      string
	Zone      string
	IsFree    bool
	Available bool
	X         int
	Y         int
}

func mockWorkspaces() []mockWorkspace {
	return []mockWorkspace{
		{ID: 1, Name: "A1", Type: "DESK", Zone: "Тихая", IsFree: true, Available: true, X: 1, Y: 1},
		{ID: 2, Name: "A2", Type: "DESK", Zone: "Тихая", IsFree: false, Available: true, X: 2, Y: 1},
		{ID: 3, Name: "A3", Type: "DESK", Zone: "Тихая", IsFree: true, Available: true, X: 3, Y: 1},
		{ID: 4, Name: "B1", Type: "DESK", Zone: "Командная", IsFree: true, Available: true, X: 1, Y: 2},
		{ID: 5, Name: "B2", Type: "DESK", Zone: "Командная", IsFree: false, Available: false, X: 2, Y: 2},
		{ID: 6, Name: "B3", Type: "DESK", Zone: "Командная", IsFree: true, Available: true, X: 3, Y: 2},
		{ID: 7, Name: "M1", Type: "MEETING_ROOM", Zone: "Переговорные", IsFree: true, Available: true, X: 1, Y: 3},
		{ID: 8, Name: "M2", Type: "MEETING_ROOM", Zone: "Переговорные", IsFree: false, Available: true, X: 2, Y: 3},
		{ID: 9, Name: "L1", Type: "LOUNGE", Zone: "Лаунж", IsFree: true, Available: true, X: 3, Y: 3},
	}
}

func schemeHandler(w http.ResponseWriter, r *http.Request) {
	render(w, "scheme.html", pageData{
		Title:     "Схема коворкинга",
		ActiveTab: "scheme",
		Data: map[string]any{
			"Workspaces": mockWorkspaces(),
			"Date":       "2025-12-01",
			"Start":      "09:00",
			"End":        "12:00",
		},
	})
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

func bookingsHandler(w http.ResponseWriter, r *http.Request) {
	render(w, "bookings.html", pageData{
		Title:      "Мои бронирования",
		ActiveTab:  "bookings",
		IsLoggedIn: true,
		UserName:   "Пользователь",
		Data: map[string]any{
			"Bookings": mockBookings(),
		},
	})
}

func adminHandler(w http.ResponseWriter, r *http.Request) {
	render(w, "admin.html", pageData{
		Title:      "Админ-панель",
		ActiveTab:  "admin",
		IsAdmin:    true,
		IsLoggedIn: true,
		UserName:   "admin@example.com",
		Data: map[string]any{
			"Workspaces":     mockWorkspaces(),
			"Bookings":       mockBookings(),
			"MaxActiveLimit": 3,
		},
	})
}
