package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"coworking/internal/auth"
	"coworking/internal/repo"
)

// reportHandler renders the admin report for a chosen [from, to] period.
// Default period is the last 30 days. The result is shown as tables and a
// simple inline bar chart for daily load.
func (a *App) reportHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось обработать форму", http.StatusBadRequest)
		return
	}
	loc := time.Local
	now := time.Now().In(loc)
	defaultFrom := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -29)
	defaultTo := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)

	fromStr := r.FormValue("from")
	toStr := r.FormValue("to")
	from := defaultFrom
	to := defaultTo

	errMsg := ""
	if fromStr != "" {
		t, err := time.ParseInLocation("2006-01-02", fromStr, loc)
		if err != nil {
			errMsg = "Некорректная дата начала"
		} else {
			from = t
		}
	} else {
		fromStr = from.Format("2006-01-02")
	}
	if toStr != "" {
		t, err := time.ParseInLocation("2006-01-02", toStr, loc)
		if err != nil {
			errMsg = "Некорректная дата окончания"
		} else {
			to = t.AddDate(0, 0, 1) // inclusive
		}
	} else {
		toStr = to.AddDate(0, 0, -1).Format("2006-01-02")
	}
	if errMsg == "" && !to.After(from) {
		errMsg = "Дата окончания должна быть не раньше даты начала"
	}

	pd := pageDataFor(r, "Отчёты", "admin")
	pd.Data = map[string]any{
		"FromStr": fromStr,
		"ToStr":   toStr,
		"Error":   errMsg,
	}

	if errMsg == "" {
		stats, err := a.Reports.Build(r.Context(), from, to)
		if err != nil {
			log.Printf("report build: %v", err)
			http.Error(w, "Не удалось построить отчёт", http.StatusInternalServerError)
			return
		}
		pd.Data["Stats"] = stats
		pd.Data["MaxDaily"] = maxDaily(stats.DailyLoad)

		if r.FormValue("save") == "1" {
			user, _ := auth.UserFrom(r.Context())
			if dataJSON, err := json.Marshal(map[string]any{
				"total":              stats.Total,
				"confirmed":          stats.Confirmed,
				"completed":          stats.Completed,
				"cancelled_by_user":  stats.CancelledByUser,
				"cancelled_by_admin": stats.CancelledByAdmin,
				"popular":            stats.PopularPlaces,
				"daily":              stats.DailyLoad,
				"users":              stats.UserActivity,
			}); err == nil {
				if err := a.Reports.Save(r.Context(), from, to, string(dataJSON), user.ID); err != nil {
					log.Printf("report save: %v", err)
					pd.Data["Error"] = "Не удалось сохранить отчёт"
				} else {
					pd.Flash = "Отчёт сохранён"
				}
			}
		}
	}

	render(w, "report.html", pd)
}

func maxDaily(days []repo.DayStat) int {
	m := 0
	for _, d := range days {
		if d.Count > m {
			m = d.Count
		}
	}
	return m
}
