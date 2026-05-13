package handlers

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"time"

	"coworking/internal/auth"
	"coworking/internal/models"
	"coworking/internal/repo"
)

// bookingCancelHandler implements UC-4 «Отмена бронирования».
//
// Validation:
//   1. user is authenticated
//   2. booking exists
//   3. booking belongs to current user
//   4. booking is currently CONFIRMED
//   5. booking has not ended yet (start may already be in the past — user can
//      interrupt an active session)
//   6. user is not in cancellation cool-down (3+ cancels in 24 h → 12 h block)
func (a *App) bookingCancelHandler(w http.ResponseWriter, r *http.Request) {
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
	id := r.FormValue("booking_id")
	if id == "" {
		http.Redirect(w, r, "/bookings?flash_err=Бронирование+не+указано", http.StatusSeeOther)
		return
	}
	b, err := a.Bookings.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrBookingNotFound) {
			http.Redirect(w, r, "/bookings?flash_err=Бронирование+не+найдено", http.StatusSeeOther)
			return
		}
		log.Printf("cancel: find: %v", err)
		http.Error(w, "Внутренняя ошибка", http.StatusInternalServerError)
		return
	}
	if b.UserID != user.ID {
		http.Error(w, "Можно отменять только свои бронирования", http.StatusForbidden)
		return
	}
	if b.Status != models.StatusConfirmed {
		http.Redirect(w, r, "/bookings?flash_err=Это+бронирование+уже+нельзя+отменить", http.StatusSeeOther)
		return
	}
	now := time.Now()
	if !b.EndTime.After(now) {
		http.Redirect(w, r, "/bookings?flash_err=Бронирование+уже+завершилось", http.StatusSeeOther)
		return
	}
	if until, err := a.Bookings.CooldownUntil(r.Context(), user.ID, now); err != nil {
		log.Printf("cancel: cooldown: %v", err)
		http.Error(w, "Внутренняя ошибка", http.StatusInternalServerError)
		return
	} else if !until.IsZero() {
		hoursLeft := int(math.Ceil(time.Until(until).Hours()))
		if hoursLeft < 1 {
			hoursLeft = 1
		}
		http.Redirect(w, r,
			"/bookings?flash_err="+url.QueryEscape(fmt.Sprintf(
				"Таймаут за частую отмену бронирований. Осталось %d час(ов).", hoursLeft)),
			http.StatusSeeOther)
		return
	}
	if err := a.Bookings.Cancel(r.Context(), b.ID, false); err != nil {
		log.Printf("cancel: update: %v", err)
		http.Error(w, "Не удалось отменить", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/bookings?flash=cancelled", http.StatusSeeOther)
}
