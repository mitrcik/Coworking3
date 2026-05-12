package handlers

import (
	"log"
	"net/http"
	"strings"

	"coworking/internal/auth"
)

// settingsHandler renders the user settings page and processes profile/password
// updates. It is mounted at /settings and requires authentication.
func (a *App) settingsHandler(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	switch r.Method {
	case http.MethodGet:
		a.renderSettings(w, r, user.FullName, user.Email, "", "")
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			a.renderSettings(w, r, user.FullName, user.Email, "Не удалось обработать форму", "")
			return
		}
		switch r.FormValue("form") {
		case "profile":
			a.handleProfileForm(w, r)
		case "password":
			a.handlePasswordForm(w, r)
		default:
			a.renderSettings(w, r, user.FullName, user.Email, "Неизвестная форма", "")
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) renderSettings(w http.ResponseWriter, r *http.Request, fullName, email, errMsg, flash string) {
	pd := pageDataFor(r, "Настройки", "settings")
	pd.Flash = flash
	pd.Data = map[string]any{
		"FullName": fullName,
		"Email":    email,
		"Error":    errMsg,
	}
	render(w, "settings.html", pd)
}

func (a *App) handleProfileForm(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	fullName := strings.TrimSpace(r.FormValue("full_name"))
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))

	if fullName == "" || email == "" {
		a.renderSettings(w, r, fullName, email, "ФИО и email обязательны", "")
		return
	}
	if msg := validateFullName(fullName); msg != "" {
		a.renderSettings(w, r, fullName, email, msg, "")
		return
	}
	if msg := validateEmail(email); msg != "" {
		a.renderSettings(w, r, fullName, email, msg, "")
		return
	}
	if email != user.Email {
		taken, err := a.Users.EmailTakenByOther(r.Context(), email, user.ID)
		if err != nil {
			log.Printf("settings: email taken check: %v", err)
			a.renderSettings(w, r, fullName, email, "Внутренняя ошибка, попробуйте позже", "")
			return
		}
		if taken {
			a.renderSettings(w, r, fullName, email, "Этот email уже занят другим пользователем", "")
			return
		}
	}
	if err := a.Users.UpdateProfile(r.Context(), user.ID, fullName, email); err != nil {
		log.Printf("settings: update profile: %v", err)
		a.renderSettings(w, r, fullName, email, "Не удалось сохранить профиль", "")
		return
	}
	a.renderSettings(w, r, fullName, email, "", "Профиль обновлён")
}

func (a *App) handlePasswordForm(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	current := r.FormValue("current_password")
	newPw := r.FormValue("new_password")
	confirm := r.FormValue("new_password_confirm")

	if current == "" || newPw == "" || confirm == "" {
		a.renderSettings(w, r, user.FullName, user.Email, "Заполните все поля смены пароля", "")
		return
	}
	if err := auth.VerifyPassword(user.PasswordHash, current); err != nil {
		a.renderSettings(w, r, user.FullName, user.Email, "Текущий пароль указан неверно", "")
		return
	}
	if len(newPw) < 4 {
		a.renderSettings(w, r, user.FullName, user.Email, "Новый пароль должен быть не короче 4 символов", "")
		return
	}
	if newPw != confirm {
		a.renderSettings(w, r, user.FullName, user.Email, "Новые пароли не совпадают", "")
		return
	}
	hash, err := auth.HashPassword(newPw)
	if err != nil {
		log.Printf("settings: hash: %v", err)
		a.renderSettings(w, r, user.FullName, user.Email, "Не удалось обновить пароль", "")
		return
	}
	if err := a.Users.UpdatePassword(r.Context(), user.ID, hash); err != nil {
		log.Printf("settings: update password: %v", err)
		a.renderSettings(w, r, user.FullName, user.Email, "Не удалось обновить пароль", "")
		return
	}
	a.renderSettings(w, r, user.FullName, user.Email, "", "Пароль изменён")
}
