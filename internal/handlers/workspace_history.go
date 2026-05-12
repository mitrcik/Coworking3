package handlers

import (
	"errors"
	"log"
	"net/http"
	"time"

	"coworking/internal/models"
	"coworking/internal/repo"
)

// workspaceHistoryView is the per-row data sent to the template.
type workspaceHistoryView struct {
	ID         string
	Date       string
	Start      string
	End        string
	Status     models.BookingStatus
	StatusText string
	IsActive   bool
}

// workspaceHistoryHandler shows the booking history for a single workspace
// (P.11). It supports a "filter" query parameter with values:
//
//	all      — all confirmed bookings
//	today    — bookings overlapping the current day
//	active   — bookings overlapping "now"
//	future   — bookings starting after now
//	past     — bookings entirely before now
func (a *App) workspaceHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось обработать форму", http.StatusBadRequest)
		return
	}
	wsID := r.FormValue("workspace_id")
	if wsID == "" {
		http.Redirect(w, r, "/scheme", http.StatusSeeOther)
		return
	}
	ws, err := a.Workspaces.FindByID(r.Context(), wsID)
	if err != nil {
		if errors.Is(err, repo.ErrWorkspaceNotFound) {
			http.Error(w, "Место не найдено", http.StatusNotFound)
			return
		}
		log.Printf("workspace history: find: %v", err)
		http.Error(w, "Внутренняя ошибка", http.StatusInternalServerError)
		return
	}

	filter := r.FormValue("filter")
	if filter == "" {
		filter = "today"
	}
	bookings, err := a.Bookings.ListByWorkspace(r.Context(), wsID, time.Time{}, time.Time{})
	if err != nil {
		log.Printf("workspace history: list: %v", err)
		http.Error(w, "Не удалось загрузить историю", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	views := make([]workspaceHistoryView, 0, len(bookings))
	for _, b := range bookings {
		// Apply filter.
		switch filter {
		case "today":
			if b.EndTime.Before(startOfDay) || !b.StartTime.Before(endOfDay) {
				continue
			}
		case "active":
			if !b.StartTime.Before(now) || !b.EndTime.After(now) {
				continue
			}
		case "future":
			if !b.StartTime.After(now) {
				continue
			}
		case "past":
			if !b.EndTime.Before(now) {
				continue
			}
		}
		v := workspaceHistoryView{
			ID:         b.ID,
			Date:       b.StartTime.Local().Format("2006-01-02"),
			Start:      b.StartTime.Local().Format("15:04"),
			End:        b.EndTime.Local().Format("15:04"),
			Status:     b.Status,
			StatusText: humanStatus(b.Status),
			IsActive:   b.StartTime.Before(now) && b.EndTime.After(now),
		}
		views = append(views, v)
	}

	pd := pageDataFor(r, "История бронирований", "scheme")
	pd.Data = map[string]any{
		"Workspace": ws,
		"Bookings":  views,
		"Filter":    filter,
		"Filters": []struct {
			Key   string
			Label string
		}{
			{"today", "Сегодня"},
			{"active", "Сейчас активные"},
			{"future", "Будущие"},
			{"past", "Прошлые"},
			{"all", "Все"},
		},
	}
	render(w, "workspace_history.html", pd)
}
