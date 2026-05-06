package handlers

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"coworking/internal/auth"
	"coworking/internal/models"
	"coworking/internal/repo"
)

// App carries shared dependencies (database repositories, sessions) for handlers.
type App struct {
	Workspaces *repo.WorkspaceRepo
	Users      *repo.UserRepo
	Bookings   *repo.BookingRepo
	Settings   *repo.SettingsRepo
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
	mux.HandleFunc("/bookings/create", a.requireUser(a.bookingCreateHandler))
	mux.HandleFunc("/bookings/cancel", a.requireUser(a.bookingCancelHandler))
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

// bookingView is the template-friendly representation of a user's booking.
type bookingView struct {
	ID            string
	Workspace     string
	Type          models.WorkspaceType
	Zone          string
	Date          string
	Start         string
	End           string
	Status        models.BookingStatus
	StatusText    string
	IsActiveNow   bool
	CanCancel     bool
}

func (a *App) bookingsHandler(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось обработать форму", http.StatusBadRequest)
		return
	}

	statusFilter := r.FormValue("status") // CONFIRMED, COMPLETED, CANCELLED, ALL
	periodFilter := r.FormValue("period") // all, future, past

	rows, err := a.Bookings.ListByUser(r.Context(), user.ID, nil)
	if err != nil {
		log.Printf("bookings: list: %v", err)
		http.Error(w, "Не удалось загрузить бронирования", http.StatusInternalServerError)
		return
	}
	now := time.Now()
	views := make([]bookingView, 0, len(rows))
	for _, b := range rows {
		// status filter
		if statusFilter != "" && statusFilter != "ALL" {
			switch statusFilter {
			case "CANCELLED":
				if b.Status != models.StatusCancelledByUser && b.Status != models.StatusCancelledByAdmin {
					continue
				}
			default:
				if string(b.Status) != statusFilter {
					continue
				}
			}
		}
		// period filter
		switch periodFilter {
		case "future":
			if !b.EndTime.After(now) {
				continue
			}
		case "past":
			if b.EndTime.After(now) {
				continue
			}
		}

		v := bookingView{
			ID:         b.ID,
			Workspace:  b.WorkspaceName,
			Type:       b.WorkspaceType,
			Zone:       b.Zone,
			Date:       b.StartTime.Local().Format("2006-01-02"),
			Start:      b.StartTime.Local().Format("15:04"),
			End:        b.EndTime.Local().Format("15:04"),
			Status:     b.Status,
			StatusText: humanStatus(b.Status),
		}
		if b.Status == models.StatusConfirmed {
			v.IsActiveNow = !b.EndTime.Before(now)
			v.CanCancel = b.StartTime.After(now) // can cancel only future bookings
		}
		views = append(views, v)
	}

	flash, flashErr := "", ""
	switch r.URL.Query().Get("flash") {
	case "created":
		flash = "Бронирование создано"
	case "cancelled":
		flash = "Бронирование отменено, место снова доступно"
	}
	if e := r.URL.Query().Get("flash_err"); e != "" {
		flashErr = e
	}

	pd := pageDataFor(r, "Мои бронирования", "bookings")
	pd.Flash = flash
	pd.Data = map[string]any{
		"Bookings":  views,
		"Status":    statusFilter,
		"Period":    periodFilter,
		"FlashErr":  flashErr,
	}
	render(w, "bookings.html", pd)
}

func humanStatus(s models.BookingStatus) string {
	switch s {
	case models.StatusConfirmed:
		return "Активно"
	case models.StatusCompleted:
		return "Завершено"
	case models.StatusCancelledByUser:
		return "Отменено пользователем"
	case models.StatusCancelledByAdmin:
		return "Отменено администратором"
	default:
		return string(s)
	}
}

func (a *App) adminHandler(w http.ResponseWriter, r *http.Request) {
	workspaces, err := a.Workspaces.List(r.Context())
	if err != nil {
		log.Printf("admin list workspaces: %v", err)
		http.Error(w, "Не удалось загрузить данные админки", http.StatusInternalServerError)
		return
	}
	allBookings, err := a.Bookings.ListAll(r.Context(), nil)
	if err != nil {
		log.Printf("admin list bookings: %v", err)
		http.Error(w, "Не удалось загрузить данные админки", http.StatusInternalServerError)
		return
	}
	settings, err := a.Settings.Get(r.Context())
	if err != nil {
		log.Printf("admin get settings: %v", err)
		http.Error(w, "Не удалось загрузить настройки", http.StatusInternalServerError)
		return
	}
	pd := pageDataFor(r, "Админ-панель", "admin")
	pd.Data = map[string]any{
		"Workspaces":     workspaces,
		"Bookings":       allBookings,
		"MaxActiveLimit": settings.MaxActiveBookingsPerUser,
	}
	render(w, "admin.html", pd)
}
