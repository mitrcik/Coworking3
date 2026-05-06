package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"coworking/internal/models"
)

// schemeWorkspaceView extends the basic workspace data with availability for
// the chosen interval.
type schemeWorkspaceView struct {
	ID         string
	Name       string
	Type       models.WorkspaceType
	Zone       string
	X          int
	Y          int
	Disabled   bool   // workspace is is_available=false (admin disabled)
	Busy       bool   // overlapping confirmed booking exists
	Selected   bool
	StatusText string // "Свободно", "Занято", "Недоступно"
	StatusKey  string // "free", "busy", "off"
}

const (
	defaultStartHour = 9
	defaultEndHour   = 12
)

// parseSchemeForm reads date / start / end / workspace_id from query/form and
// returns parsed times together with the original strings (for the form
// inputs). When values are missing or invalid we fall back to "today" with
// 09:00–12:00.
func parseSchemeForm(r *http.Request) (start, end time.Time, dateStr, startStr, endStr, workspaceID, errMsg string) {
	loc := time.Local
	now := time.Now().In(loc)
	dateStr = r.FormValue("date")
	startStr = r.FormValue("start")
	endStr = r.FormValue("end")
	workspaceID = r.FormValue("workspace_id")

	if dateStr == "" {
		dateStr = now.Format("2006-01-02")
	}
	if startStr == "" {
		startStr = pad2(defaultStartHour) + ":00"
	}
	if endStr == "" {
		endStr = pad2(defaultEndHour) + ":00"
	}

	day, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		errMsg = "Некорректная дата"
		day = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	}
	st, errS := parseHourMinute(startStr)
	en, errE := parseHourMinute(endStr)
	if errS != nil || errE != nil {
		if errMsg == "" {
			errMsg = "Некорректное время"
		}
		st, _ = parseHourMinute(pad2(defaultStartHour) + ":00")
		en, _ = parseHourMinute(pad2(defaultEndHour) + ":00")
	}
	start = time.Date(day.Year(), day.Month(), day.Day(), st.h, st.m, 0, 0, loc)
	end = time.Date(day.Year(), day.Month(), day.Day(), en.h, en.m, 0, 0, loc)

	if !end.After(start) && errMsg == "" {
		errMsg = "Время окончания должно быть позже времени начала"
	}
	// allow other handlers to redirect with ?error=
	if errMsg == "" {
		errMsg = r.FormValue("error")
	}
	return
}

type hm struct{ h, m int }

func parseHourMinute(s string) (hm, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return hm{}, err
	}
	return hm{h: t.Hour(), m: t.Minute()}, nil
}

func pad2(n int) string { return fmt.Sprintf("%02d", n) }

func (a *App) schemeHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось обработать форму", http.StatusBadRequest)
		return
	}
	start, end, dateStr, startStr, endStr, selectedID, errMsg := parseSchemeForm(r)

	workspaces, err := a.Workspaces.List(r.Context())
	if err != nil {
		log.Printf("scheme: list workspaces: %v", err)
		http.Error(w, "Не удалось загрузить схему", http.StatusInternalServerError)
		return
	}

	busyMap := map[string]struct{}{}
	if errMsg == "" {
		busyMap, err = a.Bookings.BusyWorkspaceIDs(r.Context(), start, end)
		if err != nil {
			log.Printf("scheme: busy ids: %v", err)
			http.Error(w, "Не удалось загрузить занятость", http.StatusInternalServerError)
			return
		}
	}

	views := make([]schemeWorkspaceView, 0, len(workspaces))
	var selected *schemeWorkspaceView
	for _, ws := range workspaces {
		_, busy := busyMap[ws.ID]
		v := schemeWorkspaceView{
			ID:       ws.ID,
			Name:     ws.Name,
			Type:     ws.Type,
			Zone:     ws.Zone,
			X:        ws.PositionX,
			Y:        ws.PositionY,
			Disabled: !ws.IsAvailable,
			Busy:     busy,
		}
		switch {
		case v.Disabled:
			v.StatusKey, v.StatusText = "off", "Недоступно"
		case v.Busy:
			v.StatusKey, v.StatusText = "busy", "Занято"
		default:
			v.StatusKey, v.StatusText = "free", "Свободно"
		}
		if ws.ID == selectedID {
			v.Selected = true
		}
		views = append(views, v)
		if v.Selected {
			cp := v
			selected = &cp
		}
	}

	// Validate that selected is actually selectable
	if selected != nil && (selected.Busy || selected.Disabled) {
		errMsg = "Выбранное место недоступно для бронирования. Выберите другое."
		// keep selected so user sees what was chosen
	}

	pd := pageDataFor(r, "Схема коворкинга", "scheme")
	pd.Data = map[string]any{
		"Workspaces":  views,
		"Date":        dateStr,
		"Start":       startStr,
		"End":         endStr,
		"Error":       errMsg,
		"Selected":    selected,
		"CanBook":     selected != nil && !selected.Busy && !selected.Disabled && errMsg == "",
		"FormStart":   start.Format("2006-01-02T15:04"),
		"FormEnd":     end.Format("2006-01-02T15:04"),
	}
	render(w, "scheme.html", pd)
}
