package handlers

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"coworking/internal/auth"
	"coworking/internal/models"
	"coworking/internal/repo"
)

// validWorkspaceTypes lists allowed enum values from the .md spec.
var validWorkspaceTypes = map[string]bool{
	string(models.WorkspaceDesk):        true,
	string(models.WorkspaceMeetingRoom): true,
	string(models.WorkspaceLounge):      true,
}

// adminPanelHandler renders the admin panel with workspaces, bookings and settings.
func (a *App) adminPanelHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось обработать форму", http.StatusBadRequest)
		return
	}
	workspaces, err := a.Workspaces.List(r.Context())
	if err != nil {
		log.Printf("admin list workspaces: %v", err)
		http.Error(w, "Не удалось загрузить места", http.StatusInternalServerError)
		return
	}
	settings, err := a.Settings.Get(r.Context())
	if err != nil {
		log.Printf("admin get settings: %v", err)
		http.Error(w, "Не удалось загрузить настройки", http.StatusInternalServerError)
		return
	}

	filter := repo.AdminBookingFilter{
		Status:      r.FormValue("b_status"),
		UserEmail:   strings.TrimSpace(r.FormValue("b_email")),
		WorkspaceID: r.FormValue("b_workspace"),
	}
	if df := r.FormValue("b_from"); df != "" {
		if t, err := time.ParseInLocation("2006-01-02", df, time.Local); err == nil {
			filter.From = &t
		}
	}
	if dt := r.FormValue("b_to"); dt != "" {
		if t, err := time.ParseInLocation("2006-01-02", dt, time.Local); err == nil {
			tt := t.Add(24 * time.Hour) // inclusive day
			filter.To = &tt
		}
	}
	bookings, err := a.Bookings.ListAll(r.Context(), filter)
	if err != nil {
		log.Printf("admin list bookings: %v", err)
		http.Error(w, "Не удалось загрузить бронирования", http.StatusInternalServerError)
		return
	}
	now := time.Now()
	views := make([]bookingAdminView, 0, len(bookings))
	for _, b := range bookings {
		views = append(views, bookingAdminView{
			BookingView: b,
			DateStr:     b.StartTime.Local().Format("2006-01-02"),
			StartStr:    b.StartTime.Local().Format("15:04"),
			EndStr:      b.EndTime.Local().Format("15:04"),
			StatusText:  humanStatus(b.Status),
			CanCancel:   b.Status == models.StatusConfirmed && b.StartTime.After(now),
		})
	}

	pd := pageDataFor(r, "Админ-панель", "admin")
	pd.Flash = r.URL.Query().Get("flash")
	pd.Data = map[string]any{
		"Workspaces":      workspaces,
		"Bookings":        views,
		"MaxActiveLimit":  settings.MaxActiveBookingsPerUser,
		"FlashErr":        r.URL.Query().Get("err"),
		"BookingFilter":   filter,
		"FromStr":         r.FormValue("b_from"),
		"ToStr":           r.FormValue("b_to"),
		"Statuses":        []string{"CONFIRMED", "COMPLETED", "CANCELLED_BY_USER", "CANCELLED_BY_ADMIN"},
		"WorkspaceTypes":  []string{string(models.WorkspaceDesk), string(models.WorkspaceMeetingRoom), string(models.WorkspaceLounge)},
	}
	render(w, "admin.html", pd)
}

type bookingAdminView struct {
	repo.BookingView
	DateStr    string
	StartStr   string
	EndStr     string
	StatusText string
	CanCancel  bool
}

// adminBack redirects to the admin panel with optional flash messages.
func adminBack(w http.ResponseWriter, r *http.Request, flash, errMsg string) {
	q := url.Values{}
	if flash != "" {
		q.Set("flash", flash)
	}
	if errMsg != "" {
		q.Set("err", errMsg)
	}
	target := "/admin"
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// --- workspaces -------------------------------------------------------------

func parseXY(xs, ys string) (int, int, error) {
	x, err := strconv.Atoi(strings.TrimSpace(xs))
	if err != nil {
		return 0, 0, errors.New("position_x должно быть целым числом")
	}
	y, err := strconv.Atoi(strings.TrimSpace(ys))
	if err != nil {
		return 0, 0, errors.New("position_y должно быть целым числом")
	}
	return x, y, nil
}

func (a *App) adminWorkspaceCreateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось обработать форму", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	zone := strings.TrimSpace(r.FormValue("zone"))
	wtype := strings.TrimSpace(r.FormValue("type"))
	avail := r.FormValue("is_available") == "on"
	if name == "" || zone == "" {
		adminBack(w, r, "", "Название и зона обязательны")
		return
	}
	if !validWorkspaceTypes[wtype] {
		adminBack(w, r, "", "Недопустимый тип места")
		return
	}
	x, y, err := parseXY(r.FormValue("position_x"), r.FormValue("position_y"))
	if err != nil {
		adminBack(w, r, "", err.Error())
		return
	}
	if _, err := a.Workspaces.Create(r.Context(), name, zone, models.WorkspaceType(wtype), avail, x, y); err != nil {
		log.Printf("admin create ws: %v", err)
		adminBack(w, r, "", "Не удалось создать место (возможно, имя уже используется)")
		return
	}
	adminBack(w, r, "Место добавлено", "")
}

func (a *App) adminWorkspaceUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось обработать форму", http.StatusBadRequest)
		return
	}
	id := r.FormValue("workspace_id")
	name := strings.TrimSpace(r.FormValue("name"))
	zone := strings.TrimSpace(r.FormValue("zone"))
	wtype := strings.TrimSpace(r.FormValue("type"))
	avail := r.FormValue("is_available") == "on"
	if id == "" || name == "" || zone == "" {
		adminBack(w, r, "", "Заполните все поля места")
		return
	}
	if !validWorkspaceTypes[wtype] {
		adminBack(w, r, "", "Недопустимый тип места")
		return
	}
	x, y, err := parseXY(r.FormValue("position_x"), r.FormValue("position_y"))
	if err != nil {
		adminBack(w, r, "", err.Error())
		return
	}
	if err := a.Workspaces.Update(r.Context(), id, name, zone, models.WorkspaceType(wtype), avail, x, y); err != nil {
		log.Printf("admin update ws: %v", err)
		adminBack(w, r, "", "Не удалось обновить место")
		return
	}
	adminBack(w, r, "Место обновлено", "")
}

func (a *App) adminWorkspaceToggleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.FormValue("workspace_id")
	available := r.FormValue("available") == "1"
	if err := a.Workspaces.SetAvailable(r.Context(), id, available); err != nil {
		log.Printf("admin toggle ws: %v", err)
		adminBack(w, r, "", "Не удалось изменить доступность")
		return
	}
	if available {
		adminBack(w, r, "Место включено", "")
	} else {
		adminBack(w, r, "Место отключено", "")
	}
}

func (a *App) adminWorkspaceDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.FormValue("workspace_id")
	if err := a.Workspaces.Delete(r.Context(), id, time.Now()); err != nil {
		if errors.Is(err, repo.ErrWorkspaceHasBookings) {
			adminBack(w, r, "", "Нельзя удалить место с активными будущими бронированиями")
			return
		}
		log.Printf("admin delete ws: %v", err)
		adminBack(w, r, "", "Не удалось удалить место")
		return
	}
	adminBack(w, r, "Место удалено", "")
}

// --- bookings ---------------------------------------------------------------

func (a *App) adminBookingCancelHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.FormValue("booking_id")
	if err := a.Bookings.Cancel(r.Context(), id, true); err != nil {
		if errors.Is(err, repo.ErrBookingNotFound) {
			adminBack(w, r, "", "Бронирование не найдено или уже не активно")
			return
		}
		log.Printf("admin cancel: %v", err)
		adminBack(w, r, "", "Не удалось отменить бронирование")
		return
	}
	adminBack(w, r, "Бронирование отменено администратором", "")
}

// allowed admin status transitions (no rules-violating moves).
var adminStatusTransitions = map[models.BookingStatus]map[models.BookingStatus]bool{
	models.StatusConfirmed: {
		models.StatusCompleted:         true,
		models.StatusCancelledByAdmin:  true,
	},
}

func (a *App) adminBookingStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.FormValue("booking_id")
	target := models.BookingStatus(r.FormValue("status"))
	b, err := a.Bookings.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrBookingNotFound) {
			adminBack(w, r, "", "Бронирование не найдено")
			return
		}
		log.Printf("admin status: find: %v", err)
		adminBack(w, r, "", "Не удалось найти бронирование")
		return
	}
	allowed, ok := adminStatusTransitions[b.Status]
	if !ok || !allowed[target] {
		adminBack(w, r, "", "Недопустимый переход статуса")
		return
	}
	if err := a.Bookings.UpdateStatus(r.Context(), id, target); err != nil {
		log.Printf("admin status: update: %v", err)
		adminBack(w, r, "", "Не удалось изменить статус")
		return
	}
	adminBack(w, r, "Статус бронирования изменён", "")
}

// --- settings ---------------------------------------------------------------

func (a *App) adminSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, _ := auth.UserFrom(r.Context())
	v, err := strconv.Atoi(strings.TrimSpace(r.FormValue("max_active")))
	if err != nil || v <= 0 {
		adminBack(w, r, "", "Лимит должен быть положительным целым числом")
		return
	}
	leadHours, err := strconv.Atoi(strings.TrimSpace(r.FormValue("lead_hours")))
	if err != nil || leadHours < 0 {
		adminBack(w, r, "", "minLeadHours должно быть неотрицательным целым числом")
		return
	}
	if err := a.Settings.Update(r.Context(), v, leadHours, user.ID); err != nil {
		log.Printf("admin settings update: %v", err)
		adminBack(w, r, "", "Не удалось сохранить настройки")
		return
	}
	adminBack(w, r, "Настройки сохранены", "")
}
