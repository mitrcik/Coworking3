package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"coworking/internal/auth"
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

// coworkingView is the JSON-friendly summary of a coworking used by the
// /scheme selector.
type coworkingView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	GridCols int    `json:"grid_cols"`
	GridRows int    `json:"grid_rows"`
}

// schemeEmptyCell is a (1-based) grid position that has no workspace placed on
// it. Rendered as a faint dashed placeholder so that a sparse coworking still
// reads as a grid instead of a few floating seats.
type schemeEmptyCell struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// emptyCells returns every (x, y) inside cols×rows that is not occupied by a
// workspace in views.
func emptyCells(views []schemeWorkspaceView, cols, rows int) []schemeEmptyCell {
	occupied := make(map[[2]int]bool, len(views))
	for _, v := range views {
		occupied[[2]int{v.X, v.Y}] = true
	}
	out := make([]schemeEmptyCell, 0, cols*rows-len(views))
	for y := 1; y <= rows; y++ {
		for x := 1; x <= cols; x++ {
			if !occupied[[2]int{x, y}] {
				out = append(out, schemeEmptyCell{X: x, Y: y})
			}
		}
	}
	return out
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

// schemeFormFields bundles parsed values from a /scheme or /api/scheme request.
type schemeFormFields struct {
	Start       time.Time
	End         time.Time
	DateStr     string
	StartStr    string
	EndStr      string
	DurationStr string
	WorkspaceID string
	CoworkingID string
	ErrMsg      string
}

// parseSchemeForm reads date / start / duration / end / workspace_id /
// coworking_id from the request and returns parsed times together with the
// original strings (for the form inputs). When start/end are missing we
// default to "now + 60 min".
func parseSchemeForm(r *http.Request) schemeFormFields {
	loc := time.Local
	now := time.Now().In(loc)
	f := schemeFormFields{
		DateStr:     r.FormValue("date"),
		StartStr:    r.FormValue("start"),
		EndStr:      r.FormValue("end"),
		DurationStr: r.FormValue("duration"),
		WorkspaceID: r.FormValue("workspace_id"),
		CoworkingID: r.FormValue("coworking_id"),
	}

	if f.DateStr == "" {
		f.DateStr = now.Format("2006-01-02")
	}
	if f.StartStr == "" {
		f.StartStr = now.Format("15:04")
	}
	if f.DurationStr == "" && f.EndStr == "" {
		f.DurationStr = fmt.Sprintf("%d", defaultDurationMinutes)
	}

	day, err := time.ParseInLocation("2006-01-02", f.DateStr, loc)
	if err != nil {
		f.ErrMsg = "Некорректная дата"
		day = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	}
	st, errS := parseHourMinute(f.StartStr)
	if errS != nil {
		if f.ErrMsg == "" {
			f.ErrMsg = "Некорректное время начала"
		}
		st = hm{h: now.Hour(), m: now.Minute()}
	}
	f.Start = time.Date(day.Year(), day.Month(), day.Day(), st.h, st.m, 0, 0, loc)

	durMin := defaultDurationMinutes
	if f.DurationStr != "" {
		var d int
		if _, err := fmt.Sscanf(f.DurationStr, "%d", &d); err == nil && d > 0 {
			durMin = d
		} else if f.ErrMsg == "" {
			f.ErrMsg = "Некорректная длительность"
		}
	} else if f.EndStr != "" {
		en, errE := parseHourMinute(f.EndStr)
		if errE != nil {
			if f.ErrMsg == "" {
				f.ErrMsg = "Некорректное время окончания"
			}
		} else {
			endTmp := time.Date(day.Year(), day.Month(), day.Day(), en.h, en.m, 0, 0, loc)
			if endTmp.Before(f.Start) {
				endTmp = endTmp.Add(24 * time.Hour)
			}
			diff := int(endTmp.Sub(f.Start).Minutes())
			if diff > 0 {
				durMin = diff
			}
		}
	}

	f.End = f.Start.Add(time.Duration(durMin) * time.Minute)
	f.DurationStr = fmt.Sprintf("%d", durMin)
	f.EndStr = f.End.Format("15:04")

	if !f.End.After(f.Start) && f.ErrMsg == "" {
		f.ErrMsg = "Время окончания должно быть позже времени начала"
	}
	if f.ErrMsg == "" {
		f.ErrMsg = r.FormValue("error")
	}
	return f
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
	f := parseSchemeForm(r)

	chosen, allCoworkings, err := a.coworkingList(r, f.CoworkingID)
	if err != nil {
		log.Printf("scheme: coworkings: %v", err)
		http.Error(w, "Не удалось загрузить коворкинги", http.StatusInternalServerError)
		return
	}

	var (
		views    []schemeWorkspaceView
		selected *schemeWorkspaceView
	)
	if chosen != nil {
		var busyErr error
		views, selected, busyErr = a.buildSchemeViews(r, chosen.ID, f.Start, f.End, f.WorkspaceID, f.ErrMsg != "")
		if busyErr != nil {
			log.Printf("scheme: build views: %v", busyErr)
			http.Error(w, "Не удалось загрузить схему", http.StatusInternalServerError)
			return
		}
	}

	if selected != nil && (selected.Busy || selected.Disabled) && f.ErrMsg == "" {
		f.ErrMsg = "Выбранное место недоступно для бронирования. Выберите другое."
	}

	cooldown := a.userCooldown(r)
	if cooldown.Active {
		f.ErrMsg = ""
	}

	canBook := selected != nil && !selected.Busy && !selected.Disabled && f.ErrMsg == "" && !cooldown.Active

	pd := pageDataFor(r, "Схема коворкинга", "scheme")
	pd.Data = map[string]any{
		"Workspaces":     views,
		"EmptyCells":     emptyCells(views, gridCols(chosen), gridRows(chosen)),
		"Date":           f.DateStr,
		"Start":          f.StartStr,
		"End":            f.EndStr,
		"Duration":       f.DurationStr,
		"Durations":      AllowedDurations,
		"Error":          f.ErrMsg,
		"Selected":       selected,
		"CanBook":        canBook,
		"FormStart":      f.Start.Format("2006-01-02T15:04"),
		"FormEnd":        f.End.Format("2006-01-02T15:04"),
		"MaxAhead":       MaxBookingAheadDays,
		"MaxMinutes":     MaxBookingMinutes,
		"Coworkings":     coworkingViews(allCoworkings),
		"Chosen":         chosenView(chosen),
		"GridCols":       gridCols(chosen),
		"GridRows":       gridRows(chosen),
		"Cooldown":       cooldown,
	}
	render(w, "scheme.html", pd)
}

// cooldownInfo summarises whether the current user is currently blocked from
// booking due to the cancellation throttle (3+ cancels in 24 h → 12 h block).
type cooldownInfo struct {
	Active     bool   `json:"active"`
	HoursLeft  int    `json:"hours_left"`
	Until      string `json:"until,omitempty"`
}

func (a *App) userCooldown(r *http.Request) cooldownInfo {
	user, ok := auth.UserFrom(r.Context())
	if !ok {
		return cooldownInfo{}
	}
	until, err := a.Bookings.CooldownUntil(r.Context(), user.ID, time.Now())
	if err != nil {
		log.Printf("cooldown: %v", err)
		return cooldownInfo{}
	}
	if until.IsZero() {
		return cooldownInfo{}
	}
	hoursLeft := int(math.Ceil(time.Until(until).Hours()))
	if hoursLeft < 1 {
		hoursLeft = 1
	}
	return cooldownInfo{
		Active:    true,
		HoursLeft: hoursLeft,
		Until:     until.Local().Format("2006-01-02 15:04"),
	}
}

// coworkingList wraps CoworkingRepo.List with the "fallback to first" logic.
func (a *App) coworkingList(r *http.Request, wanted string) (*models.Coworking, []models.Coworking, error) {
	list, err := a.Coworkings.List(r.Context())
	if err != nil {
		return nil, nil, err
	}
	var chosen *models.Coworking
	if wanted != "" {
		for i := range list {
			if list[i].ID == wanted {
				cw := list[i]
				chosen = &cw
				break
			}
		}
	}
	if chosen == nil && len(list) > 0 {
		cw := list[0]
		chosen = &cw
	}
	return chosen, list, nil
}

func coworkingViews(cws []models.Coworking) []coworkingView {
	out := make([]coworkingView, 0, len(cws))
	for _, c := range cws {
		out = append(out, coworkingView{
			ID:       c.ID,
			Name:     c.Name,
			GridCols: c.GridCols,
			GridRows: c.GridRows,
		})
	}
	return out
}

func chosenView(c *models.Coworking) *coworkingView {
	if c == nil {
		return nil
	}
	return &coworkingView{
		ID:       c.ID,
		Name:     c.Name,
		GridCols: c.GridCols,
		GridRows: c.GridRows,
	}
}

func gridCols(c *models.Coworking) int {
	if c == nil {
		return 3
	}
	return c.GridCols
}

func gridRows(c *models.Coworking) int {
	if c == nil {
		return 3
	}
	return c.GridRows
}

// buildSchemeViews assembles the per-seat data + selected-seat info for one
// coworking. The `skipBusy` flag is honoured so callers that already detected
// a form error can avoid the extra database query.
func (a *App) buildSchemeViews(r *http.Request, coworkingID string, start, end time.Time, selectedID string, skipBusy bool) ([]schemeWorkspaceView, *schemeWorkspaceView, error) {
	workspaces, err := a.Workspaces.ListByCoworking(r.Context(), coworkingID)
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
	EmptyCells []schemeEmptyCell     `json:"empty_cells"`
	Coworking  *coworkingView        `json:"coworking,omitempty"`
	Coworkings []coworkingView       `json:"coworkings"`
	Cooldown   cooldownInfo          `json:"cooldown"`
}

// schemeAPIHandler returns the same scheme data as /scheme but as JSON so the
// page can refresh dynamically without a full reload.
func (a *App) schemeAPIHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Не удалось обработать форму"})
		return
	}
	f := parseSchemeForm(r)
	chosen, allCoworkings, err := a.coworkingList(r, f.CoworkingID)
	if err != nil {
		log.Printf("scheme api: coworkings: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Не удалось загрузить коворкинги"})
		return
	}
	var (
		views    []schemeWorkspaceView
		selected *schemeWorkspaceView
	)
	if chosen != nil {
		views, selected, err = a.buildSchemeViews(r, chosen.ID, f.Start, f.End, f.WorkspaceID, f.ErrMsg != "")
		if err != nil {
			log.Printf("scheme api: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Не удалось загрузить схему"})
			return
		}
	}
	if selected != nil && (selected.Busy || selected.Disabled) && f.ErrMsg == "" {
		f.ErrMsg = "Выбранное место недоступно для бронирования. Выберите другое."
	}
	cooldown := a.userCooldown(r)
	if cooldown.Active {
		f.ErrMsg = ""
	}
	canBook := selected != nil && !selected.Busy && !selected.Disabled && f.ErrMsg == "" && !cooldown.Active
	writeJSON(w, http.StatusOK, schemeAPIResponse{
		Date:       f.DateStr,
		Start:      f.StartStr,
		End:        f.EndStr,
		Duration:   f.DurationStr,
		Error:      f.ErrMsg,
		CanBook:    canBook,
		Selected:   selected,
		Workspaces: views,
		EmptyCells: emptyCells(views, gridCols(chosen), gridRows(chosen)),
		Coworking:  chosenView(chosen),
		Coworkings: coworkingViews(allCoworkings),
		Cooldown:   cooldown,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}




