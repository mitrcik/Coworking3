package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode"

	"coworking/internal/auth"
	"coworking/internal/models"
	"coworking/internal/repo"
)

type formError struct {
	Field   string
	Message string
}

// validateEmail returns a human-readable Russian error string if email is
// invalid, or an empty string when the email is acceptable. It produces short
// hints aligned with UX feedback: only mentions @, missing domain part, and
// non-latin characters. An empty email is considered handled elsewhere.
func validateEmail(email string) string {
	if email == "" {
		return ""
	}
	for _, r := range email {
		if unicode.Is(unicode.Cyrillic, r) {
			return "Неверно указан адрес почты, пишите только латинскими буквами"
		}
	}
	at := strings.IndexByte(email, '@')
	if at < 0 {
		return "Адрес почты должен содержать @"
	}
	local := email[:at]
	domain := email[at+1:]
	if local == "" {
		return "Адрес почты должен содержать часть до символа @"
	}
	if domain == "" {
		return "Введённый адрес почты не полный. Укажите часть после символа @"
	}
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "Введённый адрес почты не полный. Укажите часть после символа @"
	}
	// Allow only latin letters, digits and a few punctuation chars; cyrillic
	// already rejected above.
	for _, r := range email {
		if r > unicode.MaxASCII {
			return "Неверно указан адрес почты, пишите только латинскими буквами"
		}
	}
	return ""
}

// validateFullName checks for digits and minimum content.
func validateFullName(name string) string {
	if name == "" {
		return ""
	}
	for _, r := range name {
		if unicode.IsDigit(r) {
			return "ФИО не должно содержать цифры"
		}
	}
	return ""
}

func (a *App) renderLogin(w http.ResponseWriter, email, errMsg, flash string) {
	render(w, "login.html", pageData{
		Title:     "Вход",
		ActiveTab: "auth",
		Flash:     flash,
		Data: map[string]any{
			"Email":  email,
			"Error":  errMsg,
		},
	})
}

func (a *App) loginHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.renderLogin(w, "", "", "")
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			a.renderLogin(w, "", "Не удалось обработать форму", "")
			return
		}
		email := strings.TrimSpace(r.FormValue("email"))
		password := r.FormValue("password")

		if email == "" || password == "" {
			a.renderLogin(w, email, "Заполните email и пароль", "")
			return
		}
		if msg := validateEmail(email); msg != "" {
			a.renderLogin(w, email, msg, "")
			return
		}

		user, err := a.Users.FindByEmail(r.Context(), strings.ToLower(email))
		if err != nil {
			if errors.Is(err, repo.ErrUserNotFound) {
				a.renderLogin(w, email, "Неверный email или пароль", "")
				return
			}
			log.Printf("login: find user: %v", err)
			a.renderLogin(w, email, "Внутренняя ошибка, попробуйте позже", "")
			return
		}
		if err := auth.VerifyPassword(user.PasswordHash, password); err != nil {
			a.renderLogin(w, email, "Неверный email или пароль", "")
			return
		}
		if err := a.Sessions.Login(w, user.ID); err != nil {
			log.Printf("login: create session: %v", err)
			a.renderLogin(w, email, "Не удалось создать сессию", "")
			return
		}
		// admins go straight to admin panel
		if user.Role == models.RoleAdmin {
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/bookings", http.StatusSeeOther)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) renderRegister(w http.ResponseWriter, fullName, email, errMsg string) {
	render(w, "register.html", pageData{
		Title:     "Регистрация",
		ActiveTab: "auth",
		Data: map[string]any{
			"FullName": fullName,
			"Email":    email,
			"Error":    errMsg,
		},
	})
}

func (a *App) registerHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.renderRegister(w, "", "", "")
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			a.renderRegister(w, "", "", "Не удалось обработать форму")
			return
		}
		fullName := strings.TrimSpace(r.FormValue("full_name"))
		email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
		pw := r.FormValue("password")
		pwc := r.FormValue("password_confirm")

		if fullName == "" || email == "" || pw == "" || pwc == "" {
			a.renderRegister(w, fullName, email, "Все поля обязательны")
			return
		}
		if msg := validateFullName(fullName); msg != "" {
			a.renderRegister(w, fullName, email, msg)
			return
		}
		if msg := validateEmail(email); msg != "" {
			a.renderRegister(w, fullName, email, msg)
			return
		}
		if len(pw) < 4 {
			a.renderRegister(w, fullName, email, "Пароль должен быть не короче 4 символов")
			return
		}
		if pw != pwc {
			a.renderRegister(w, fullName, email, "Пароли не совпадают")
			return
		}
		exists, err := a.Users.EmailExists(r.Context(), email)
		if err != nil {
			log.Printf("register: email exists: %v", err)
			a.renderRegister(w, fullName, email, "Внутренняя ошибка, попробуйте позже")
			return
		}
		if exists {
			a.renderRegister(w, fullName, email, "Пользователь с таким email уже существует")
			return
		}

		hash, err := auth.HashPassword(pw)
		if err != nil {
			log.Printf("register: hash: %v", err)
			a.renderRegister(w, fullName, email, "Не удалось создать аккаунт")
			return
		}
		uid, err := a.Users.Create(r.Context(), fullName, email, hash, models.RoleUser)
		if err != nil {
			log.Printf("register: create user: %v", err)
			a.renderRegister(w, fullName, email, "Не удалось создать аккаунт")
			return
		}
		if err := a.Sessions.Login(w, uid); err != nil {
			log.Printf("register: login: %v", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/bookings", http.StatusSeeOther)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) logoutHandler(w http.ResponseWriter, r *http.Request) {
	a.Sessions.Logout(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// withUser is a middleware that loads the current user (if authenticated) and
// attaches them to the request context. It does not enforce auth — that is
// done by requireUser / requireAdmin.
func (a *App) withUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if uid, ok := a.Sessions.CurrentUserID(r); ok {
			if u, err := a.Users.FindByID(r.Context(), uid); err == nil {
				r = r.WithContext(auth.WithUser(r.Context(), u))
			}
		}
		next(w, r)
	}
}

func (a *App) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return a.withUser(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.UserFrom(r.Context()); !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	})
}

func (a *App) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return a.withUser(func(w http.ResponseWriter, r *http.Request) {
		u, ok := auth.UserFrom(r.Context())
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if u.Role != models.RoleAdmin {
			http.Error(w, "Доступ запрещён: требуется роль администратора", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}
