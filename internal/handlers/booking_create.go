package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"coworking/internal/auth"
	"coworking/internal/repo"
)

// bookingCreateHandler implements UC-3 «Бронирование места».
//
// Validation order (matches .md spec):
//   1. user is authenticated
//   2. date / start / end are valid
//   3. end > start
//   4. interval is not entirely in the past
//   5. workspace exists
//   6. workspace is available (is_available)
//   7. no other CONFIRMED booking overlaps for this workspace
//   8. user has not exceeded the active-bookings limit
//   9. user has no other CONFIRMED booking overlapping the same interval
func (a *App) bookingCreateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось обработать форму", http.StatusBadRequest)
		return
	}

	user, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	wsID := r.FormValue("workspace_id")
	dateStr := r.FormValue("date")
	startStr := r.FormValue("start")
	endStr := r.FormValue("end")
	durationStr := r.FormValue("duration")

	loc := time.Local
	day, derr := time.ParseInLocation("2006-01-02", dateStr, loc)
	stHM, sterr := parseHourMinute(startStr)
	if wsID == "" || derr != nil || sterr != nil {
		redirectScheme(w, r, dateStr, startStr, endStr, wsID, "Заполните все поля бронирования")
		return
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), stHM.h, stHM.m, 0, 0, loc)

	// Resolve end: prefer explicit duration; fall back to end time field.
	var end time.Time
	if durationStr != "" {
		var dMin int
		if _, err := fmt.Sscanf(durationStr, "%d", &dMin); err != nil || dMin <= 0 {
			redirectScheme(w, r, dateStr, startStr, endStr, wsID, "Некорректная длительность бронирования")
			return
		}
		end = start.Add(time.Duration(dMin) * time.Minute)
		endStr = end.Format("15:04")
	} else {
		enHM, enerr := parseHourMinute(endStr)
		if enerr != nil {
			redirectScheme(w, r, dateStr, startStr, endStr, wsID, "Заполните все поля бронирования")
			return
		}
		end = time.Date(day.Year(), day.Month(), day.Day(), enHM.h, enHM.m, 0, 0, loc)
	}

	if !end.After(start) {
		redirectScheme(w, r, dateStr, startStr, endStr, wsID, "Время окончания должно быть позже времени начала")
		return
	}
	// P.12 — single booking ≤ MaxBookingMinutes.
	durationMinutes := int(end.Sub(start).Minutes())
	if durationMinutes > MaxBookingMinutes {
		redirectScheme(w, r, dateStr, startStr, endStr, wsID,
			fmt.Sprintf("Бронировать можно не больше %d часов подряд", MaxBookingMinutes/60))
		return
	}
	now := time.Now()
	if end.Before(now) {
		redirectScheme(w, r, dateStr, startStr, endStr, wsID, "Нельзя бронировать в прошлом")
		return
	}
	// P.12 — booking can only start up to MaxBookingAheadDays in the future.
	maxAhead := now.Add(time.Duration(MaxBookingAheadDays) * 24 * time.Hour)
	if start.After(maxAhead) {
		redirectScheme(w, r, dateStr, startStr, endStr, wsID,
			fmt.Sprintf("Бронировать можно не дальше чем на %d дня вперёд", MaxBookingAheadDays))
		return
	}

	ws, err := a.Workspaces.FindByID(r.Context(), wsID)
	if err != nil {
		if errors.Is(err, repo.ErrWorkspaceNotFound) {
			redirectScheme(w, r, dateStr, startStr, endStr, "", "Место не найдено")
			return
		}
		log.Printf("booking create: find workspace: %v", err)
		http.Error(w, "Внутренняя ошибка", http.StatusInternalServerError)
		return
	}
	if !ws.IsAvailable {
		redirectScheme(w, r, dateStr, startStr, endStr, wsID, "Место отключено администратором")
		return
	}

	conflict, err := a.Bookings.HasConflict(r.Context(), ws.ID, start, end)
	if err != nil {
		log.Printf("booking create: has conflict: %v", err)
		http.Error(w, "Внутренняя ошибка", http.StatusInternalServerError)
		return
	}
	if conflict {
		redirectScheme(w, r, dateStr, startStr, endStr, wsID, "Это место уже занято на выбранный интервал")
		return
	}

	settings, err := a.Settings.Get(r.Context())
	if err != nil {
		log.Printf("booking create: get settings: %v", err)
		http.Error(w, "Внутренняя ошибка", http.StatusInternalServerError)
		return
	}
	active, err := a.Bookings.CountActiveByUser(r.Context(), user.ID, time.Now())
	if err != nil {
		log.Printf("booking create: count active: %v", err)
		http.Error(w, "Внутренняя ошибка", http.StatusInternalServerError)
		return
	}
	if active >= settings.MaxActiveBookingsPerUser {
		redirectScheme(w, r, dateStr, startStr, endStr, wsID,
			"Превышен лимит активных бронирований ("+strconv.Itoa(settings.MaxActiveBookingsPerUser)+")")
		return
	}

	userConflict, err := a.Bookings.HasUserConflict(r.Context(), user.ID, start, end)
	if err != nil {
		log.Printf("booking create: user conflict: %v", err)
		http.Error(w, "Внутренняя ошибка", http.StatusInternalServerError)
		return
	}
	if userConflict {
		redirectScheme(w, r, dateStr, startStr, endStr, wsID,
			"У вас уже есть бронирование на пересекающийся интервал")
		return
	}

	if _, err := a.Bookings.Create(r.Context(), user.ID, ws.ID, start, end); err != nil {
		log.Printf("booking create: insert: %v", err)
		http.Error(w, "Не удалось создать бронирование", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/bookings?flash=created", http.StatusSeeOther)
}

// redirectScheme returns the user back to /scheme keeping the chosen
// date/time/workspace and showing an error message via query string.
func redirectScheme(w http.ResponseWriter, r *http.Request, date, start, end, wsID, msg string) {
	q := url.Values{}
	if date != "" {
		q.Set("date", date)
	}
	if start != "" {
		q.Set("start", start)
	}
	if end != "" {
		q.Set("end", end)
	}
	if wsID != "" {
		q.Set("workspace_id", wsID)
	}
	if msg != "" {
		q.Set("error", msg)
	}
	http.Redirect(w, r, "/scheme?"+q.Encode(), http.StatusSeeOther)
}

