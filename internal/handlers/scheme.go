package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"coworking/internal/models"
)

// schemeWorkspaceView extends the basic workspace data with availability for
// the chosen interval.
type schemeWorkspaceView struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Type       models.WorkspaceType `json:"type"`
	Zone       string               `json:"zone"`
	X          int                  `json:"x"`
	Y          int                  `json:"y"`
	Disabled   bool                 `json:"disabled"`
	Busy       bool                 `json:"busy"`
	Selected   bool                 `json:"selected"`
	StatusText string               `json:"status_text"`
	StatusKey  string               `json:"status_key"`
	BusyLabel  string               `json:"busy_label,omitempty"`
	BusyDate   string               `json:"busy_date,omitempty"`
}

// AllowedDurations is the fixed set of selectable booking durations (minutes).
// Used both in the UI (radio buttons) and on the server when validating.
var AllowedDurations = []int{5, 15, 30, 45, 60, 90, 120, 180}

// MaxBookingMinutes is the upper bound on a single booking (P.12).
const MaxBookingMinutes = 180

// MaxBookingAheadDays is the upper bound (in days) on how far into the future
// a booking can start (P.12).
const MaxBookingAheadDays = 3

// MaxConcurrentBookings is the per-user limit for active (current + future)
// bookings (P.12). The settings table stays as the source of truth, but new
// installs default to this value.
const MaxConcurrentBookings = 3

// defaultDurationMinutes is the duration pre-selected when the user opens
// /scheme without explicit query params.
const defaultDurationMinutes = 60

// parseSchemeForm reads date / start / duration / end / workspace_id from
// query/form and returns parsed times together with the original strings (for
// the form inputs). When start/end are missing we default to "now + 60min".
func parseSchemeForm(r *http.Request) (start, end time.Time, dateStr, startStr, endStr, durationStr, workspaceID, errMsg string) {
	loc := time.Local
	now := time.Now().In(loc)
	dateStr = r.FormValue("date")
	startStr = r.FormValue("start")
	endStr = r.FormValue("end")
	durationStr = r.FormValue("duration")
	workspaceID = r.FormValue("workspace_id")

	// Default to current time (P.7).
	if dateStr == "" {
		dateStr = now.Format("2006-01-02")
	}
	if startStr == "" {
		startStr = now.Format("15:04")
	}
	if durationStr == "" && endStr == "" {
		durationStr = fmt.Sprintf("%d", defaultDurationMinutes)
	}

	day, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		errMsg = "Некорректная дата"
		day = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	}
	st, errS := parseHourMinute(startStr)
	if errS != nil {
		if errMsg == "" {
			errMsg = "Некорректное время начала"
		}
		st = hm{h: now.Hour(), m: now.Minute()}
	}
	start = time.Date(day.Year(), day.Month(), day.Day(), st.h, st.m, 0, 0, loc)

	// Prefer the explicit duration parameter; fall back to the end time when
	// supplied. This keeps backwards compatibility for old hand-typed URLs.
	durMin := defaultDurationMinutes
	if durationStr != "" {
		var d int
		if _, err := fmt.Sscanf(durationStr, "%d", &d); err == nil && d > 0 {
			durMin = d
		} else if errMsg == "" {
			errMsg = "Некорректная длительность"
		}
	} else if endStr != "" {
		en, errE := parseHourMinute(endStr)
		if errE != nil {
			if errMsg == "" {
				errMsg = "Некорректное время окончания"
			}
		} else {
			endTmp := time.Date(day.Year(), day.Month(), day.Day(), en.h, en.m, 0, 0, loc)
			if endTmp.Before(start) {
				endTmp = endTmp.Add(24 * time.Hour)
			}
			diff := int(endTmp.Sub(start).Minutes())
			if diff > 0 {
				durMin = diff
			}
		}
	}

	end = start.Add(time.Duration(durMin) * time.Minute)
	durationStr = fmt.Sprintf("%d", durMin)
	endStr = end.Format("15:04")

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
	start, end, dateStr, startStr, endStr, durationStr, selectedID, errMsg := parseSchemeForm(r)

	views, selected, busyErr := a.buildSchemeViews(r, start, end, selectedID, errMsg != "")
	if busyErr != nil {
		log.Printf("scheme: build views: %v", busyErr)
		http.Error(w, "Не удалось загрузить схему", http.StatusInternalServerError)
		return
	}

	// Validate that selected is actually selectable
	if selected != nil && (selected.Busy || selected.Disabled) && errMsg == "" {
		errMsg = "Выбранное место недоступно для бронирования. Выберите другое."
		// keep selected so user sees what was chosen
	}

	pd := pageDataFor(r, "Схема коворкинга", "scheme")
	pd.Data = map[string]any{
		"Workspaces": views,
		"Date":       dateStr,
		"Start":      startStr,
		"End":        endStr,
		"Duration":   durationStr,
		"Durations":  AllowedDurations,
		"Error":      errMsg,
		"Selected":   selected,
		"CanBook":    selected != nil && !selected.Busy && !selected.Disabled && errMsg == "",
		"FormStart":  start.Format("2006-01-02T15:04"),
		"FormEnd":    end.Format("2006-01-02T15:04"),
		"MaxAhead":   MaxBookingAheadDays,
		"MaxMinutes": MaxBookingMinutes,
	}
	render(w, "scheme.html", pd)
}

// buildSchemeViews assembles the per-seat data + selected-seat info. The
// `skipBusy` flag is honoured so callers that already detected a form error
// can avoid the extra database query.
func (a *App) buildSchemeViews(r *http.Request, start, end time.Time, selectedID string, skipBusy bool) ([]schemeWorkspaceView, *schemeWorkspaceView, error) {
	workspaces, err := a.Workspaces.List(r.Context())
	if err != nil {
		return nil, nil, fmt.Errorf("list workspaces: %w", err)
	}
	busyMap := map[string]busyInfo{}
	if !skipBusy {
		rows, err := a.Bookings.BusyDetailsForWorkspaces(r.Context(), start, end)
		if err != nil {
			return nil, nil, fmt.Errorf("busy details: %w", err)
		}
		for _, b := range rows {
			busyMap[b.WorkspaceID] = busyInfo{
				start: b.StartTime,
				end:   b.EndTime,
			}
		}
	}
	views := make([]schemeWorkspaceView, 0, len(workspaces))
	var selected *schemeWorkspaceView
	for _, ws := range workspaces {
		info, busy := busyMap[ws.ID]
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
			v.BusyLabel = "до " + info.end.Local().Format("15:04")
			v.BusyDate = info.start.Local().Format("2006-01-02")
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
	return views, selected, nil
}

type busyInfo struct {
	start time.Time
	end   time.Time
}

// schemeAPIResponse is the JSON shape returned by /api/scheme.
type schemeAPIResponse struct {
	Date       string                `json:"date"`
	Start      string                `json:"start"`
	End        string                `json:"end"`
	Duration   string                `json:"duration"`
	Error      string                `json:"error,omitempty"`
	CanBook    bool                  `json:"can_book"`
	Selected   *schemeWorkspaceView  `json:"selected,omitempty"`
	Workspaces []schemeWorkspaceView `json:"workspaces"`
}

// schemeAPIHandler returns the same scheme data as /scheme but as JSON so the
// page can refresh dynamically without a full reload (P.1).
func (a *App) schemeAPIHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Не удалось обработать форму"})
		return
	}
	start, end, dateStr, startStr, endStr, durationStr, selectedID, errMsg := parseSchemeForm(r)
	views, selected, err := a.buildSchemeViews(r, start, end, selectedID, errMsg != "")
	if err != nil {
		log.Printf("scheme api: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Не удалось загрузить схему"})
		return
	}
	if selected != nil && (selected.Busy || selected.Disabled) && errMsg == "" {
		errMsg = "Выбранное место недоступно для бронирования. Выберите другое."
	}
	writeJSON(w, http.StatusOK, schemeAPIResponse{
		Date:       dateStr,
		Start:      startStr,
		End:        endStr,
		Duration:   durationStr,
		Error:      errMsg,
		CanBook:    selected != nil && !selected.Busy && !selected.Disabled && errMsg == "",
		Selected:   selected,
		Workspaces: views,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
