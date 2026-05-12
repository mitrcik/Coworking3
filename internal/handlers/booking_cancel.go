package handlers

import (
	"errors"
	"log"
	"net/http"
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
//   5. booking start is still in the future
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
	if !b.StartTime.After(time.Now()) {
		http.Redirect(w, r, "/bookings?flash_err=Бронирование+уже+началось+или+завершилось", http.StatusSeeOther)
		return
	}
	if err := a.Bookings.Cancel(r.Context(), b.ID, false); err != nil {
		log.Printf("cancel: update: %v", err)
		http.Error(w, "Не удалось отменить", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/bookings?flash=cancelled", http.StatusSeeOther)
}
