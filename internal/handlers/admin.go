package handlers

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"coworking/internal/auth"
	"coworking/internal/models"
	"coworking/internal/repo"
)

// validWorkspaceTypes lists allowed enum values from the .md spec.
var validWorkspaceTypes = map[string]bool{
	string(models.WorkspaceDesk):        true,
	string(models.WorkspaceMeetingRoom): true,
	string(models.WorkspaceLounge):      true,
}

// adminPanelHandler renders the admin panel with coworkings, workspaces,
// bookings and settings. The admin can switch the active coworking via
// `coworking_id` query param; the workspace CRUD section is scoped to that
// coworking.
func (a *App) adminPanelHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось обработать форму", http.StatusBadRequest)
		return
	}
	if _, err := a.Bookings.MarkPastCompleted(r.Context(), time.Now()); err != nil {
		log.Printf("admin: mark past completed: %v", err)
	}
	coworkings, err := a.Coworkings.List(r.Context())
	if err != nil {
		log.Printf("admin list coworkings: %v", err)
		http.Error(w, "Не удалось загрузить коворкинги", http.StatusInternalServerError)
		return
	}
	wanted := r.FormValue("coworking_id")
	var chosen *models.Coworking
	for i := range coworkings {
		if coworkings[i].ID == wanted {
			c := coworkings[i]
			chosen = &c
			break
		}
	}
	if chosen == nil && len(coworkings) > 0 {
		c := coworkings[0]
		chosen = &c
	}
	var workspaces []models.Workspace
	if chosen != nil {
		workspaces, err = a.Workspaces.ListByCoworking(r.Context(), chosen.ID)
		if err != nil {
			log.Printf("admin list workspaces: %v", err)
			http.Error(w, "Не удалось загрузить места", http.StatusInternalServerError)
			return
		}
	}
	allWorkspaces, err := a.Workspaces.List(r.Context())
	if err != nil {
		log.Printf("admin list all workspaces: %v", err)
		http.Error(w, "Не удалось загрузить места", http.StatusInternalServerError)
		return
	}
	settings, err := a.Settings.Get(r.Context())
	if err != nil {
		log.Printf("admin get settings: %v", err)
		http.Error(w, "Не удалось загрузить настройки", http.StatusInternalServerError)
		return
	}

	filter := repo.AdminBookingFilter{
		Status:      r.FormValue("b_status"),
		UserEmail:   strings.TrimSpace(r.FormValue("b_email")),
		WorkspaceID: r.FormValue("b_workspace"),
	}
	if df := r.FormValue("b_from"); df != "" {
		if t, err := time.ParseInLocation("2006-01-02", df, time.Local); err == nil {
			filter.From = &t
		}
	}
	if dt := r.FormValue("b_to"); dt != "" {
		if t, err := time.ParseInLocation("2006-01-02", dt, time.Local); err == nil {
			tt := t.Add(24 * time.Hour) // inclusive day
			filter.To = &tt
		}
	}
	bookings, err := a.Bookings.ListAll(r.Context(), filter)
	if err != nil {
		log.Printf("admin list bookings: %v", err)
		http.Error(w, "Не удалось загрузить бронирования", http.StatusInternalServerError)
		return
	}
	now := time.Now()
	views := make([]bookingAdminView, 0, len(bookings))
	for _, b := range bookings {
		views = append(views, bookingAdminView{
			BookingView: b,
			DateStr:     b.StartTime.Local().Format("2006-01-02"),
			StartStr:    b.StartTime.Local().Format("15:04"),
			EndStr:      b.EndTime.Local().Format("15:04"),
			StatusText:  humanStatus(b.Status),
			CanCancel:   b.Status == models.StatusConfirmed && b.StartTime.After(now),
		})
	}

	// Build a 2D grid layout for the chosen coworking so the template can
	// render the spatial map of workspaces with empty cells marked clearly.
	var grid [][]adminGridCell
	if chosen != nil {
		index := map[[2]int]*models.Workspace{}
		for i := range workspaces {
			w := &workspaces[i]
			index[[2]int{w.PositionX, w.PositionY}] = w
		}
		grid = make([][]adminGridCell, chosen.GridRows)
		for y := 1; y <= chosen.GridRows; y++ {
			row := make([]adminGridCell, chosen.GridCols)
			for x := 1; x <= chosen.GridCols; x++ {
				cell := adminGridCell{X: x, Y: y}
				if w, ok := index[[2]int{x, y}]; ok {
					cell.Workspace = w
				}
				row[x-1] = cell
			}
			grid[y-1] = row
		}
	}

	pd := pageDataFor(r, "Админ-панель", "admin")
	pd.Flash = r.URL.Query().Get("flash")
	pd.Data = map[string]any{
		"Coworkings":      coworkings,
		"Chosen":          chosen,
		"Workspaces":      workspaces,
		"AllWorkspaces":   allWorkspaces,
		"Grid":            grid,
		"Bookings":        views,
		"MaxActiveLimit":  settings.MaxActiveBookingsPerUser,
		"FlashErr":        r.URL.Query().Get("err"),
		"BookingFilter":   filter,
		"FromStr":         r.FormValue("b_from"),
		"ToStr":           r.FormValue("b_to"),
		"Statuses":        []string{"CONFIRMED", "COMPLETED", "CANCELLED_BY_USER", "CANCELLED_BY_ADMIN"},
		"WorkspaceTypes":  []string{string(models.WorkspaceDesk), string(models.WorkspaceMeetingRoom), string(models.WorkspaceLounge)},
	}
	render(w, "admin.html", pd)
}

// adminGridCell describes one (x,y) slot in the chosen coworking's grid.
// `Workspace` is nil when the cell is free (admin can place a new place there).
type adminGridCell struct {
	X         int
	Y         int
	Workspace *models.Workspace
}

type bookingAdminView struct {
	repo.BookingView
	DateStr    string
	StartStr   string
	EndStr     string
	StatusText string
	CanCancel  bool
}

// adminBack redirects to the admin panel with optional flash messages.
// If a `coworking_id` is provided (either via form or query string), it is
// preserved so the user lands back on the same coworking after a CRUD action.
func adminBack(w http.ResponseWriter, r *http.Request, flash, errMsg string) {
	q := url.Values{}
	if flash != "" {
		q.Set("flash", flash)
	}
	if errMsg != "" {
		q.Set("err", errMsg)
	}
	if cw := strings.TrimSpace(r.FormValue("coworking_id")); cw != "" {
		q.Set("coworking_id", cw)
	}
	target := "/admin"
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// --- workspaces -------------------------------------------------------------

func parseXY(xs, ys string) (int, int, error) {
	x, err := strconv.Atoi(strings.TrimSpace(xs))
	if err != nil {
		return 0, 0, errors.New("position_x должно быть целым числом")
	}
	y, err := strconv.Atoi(strings.TrimSpace(ys))
	if err != nil {
		return 0, 0, errors.New("position_y должно быть целым числом")
	}
	return x, y, nil
}

// classifyWorkspaceUserError maps repository errors to localized admin
// messages. Anything we don’t recognise becomes a generic 500-style message.
func classifyWorkspaceUserError(err error) string {
	switch {
	case errors.Is(err, repo.ErrWorkspaceNameTaken):
		return "Это название уже используется в этом коворкинге"
	case errors.Is(err, repo.ErrPositionTaken):
		return "Эта позиция уже занята другим местом"
	case errors.Is(err, repo.ErrPositionOutOfGrid):
		return "Координаты выходят за границы сетки коворкинга"
	case errors.Is(err, repo.ErrCoworkingNotFound):
		return "Коворкинг не найден"
	default:
		return ""
	}
}

// resolveCoworkingForCreate validates the form-supplied coworking id against
// the active coworking list and returns its grid size. The size is used to
// check that (x,y) fits within the grid.
func (a *App) resolveCoworkingForCreate(r *http.Request) (*models.Coworking, error) {
	id := strings.TrimSpace(r.FormValue("coworking_id"))
	if id == "" {
		return nil, repo.ErrCoworkingNotFound
	}
	cw, err := a.Coworkings.FindByID(r.Context(), id)
	if err != nil {
		return nil, err
	}
	return cw, nil
}

func (a *App) adminWorkspaceCreateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось обработать форму", http.StatusBadRequest)
		return
	}
	cw, err := a.resolveCoworkingForCreate(r)
	if err != nil {
		adminBack(w, r, "", "Выберите коворкинг")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	zone := strings.TrimSpace(r.FormValue("zone"))
	wtype := strings.TrimSpace(r.FormValue("type"))
	avail := r.FormValue("is_available") == "on"
	if name == "" || zone == "" {
		adminBack(w, r, "", "Название и зона обязательны")
		return
	}
	if !validWorkspaceTypes[wtype] {
		adminBack(w, r, "", "Недопустимый тип места")
		return
	}
	x, y, err := parseXY(r.FormValue("position_x"), r.FormValue("position_y"))
	if err != nil {
		adminBack(w, r, "", err.Error())
		return
	}
	if x < 1 || y < 1 || x > cw.GridCols || y > cw.GridRows {
		adminBack(w, r, "", "Координаты выходят за границы сетки коворкинга")
		return
	}
	if _, err := a.Workspaces.Create(r.Context(), cw.ID, name, zone, models.WorkspaceType(wtype), avail, x, y); err != nil {
		if msg := classifyWorkspaceUserError(err); msg != "" {
			adminBack(w, r, "", msg)
			return
		}
		log.Printf("admin create ws: %v", err)
		adminBack(w, r, "", "Не удалось создать место")
		return
	}
	adminBack(w, r, "Место добавлено", "")
}

func (a *App) adminWorkspaceUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось обработать форму", http.StatusBadRequest)
		return
	}
	id := r.FormValue("workspace_id")
	name := strings.TrimSpace(r.FormValue("name"))
	zone := strings.TrimSpace(r.FormValue("zone"))
	wtype := strings.TrimSpace(r.FormValue("type"))
	avail := r.FormValue("is_available") == "on"
	if id == "" || name == "" || zone == "" {
		adminBack(w, r, "", "Заполните все поля места")
		return
	}
	if !validWorkspaceTypes[wtype] {
		adminBack(w, r, "", "Недопустимый тип места")
		return
	}
	x, y, err := parseXY(r.FormValue("position_x"), r.FormValue("position_y"))
	if err != nil {
		adminBack(w, r, "", err.Error())
		return
	}
	// Load existing workspace to know its coworking + verify grid bounds.
	existing, err := a.Workspaces.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrWorkspaceNotFound) {
			adminBack(w, r, "", "Место не найдено")
			return
		}
		log.Printf("admin update ws: find: %v", err)
		adminBack(w, r, "", "Не удалось обновить место")
		return
	}
	cw, err := a.Coworkings.FindByID(r.Context(), existing.CoworkingID)
	if err != nil {
		log.Printf("admin update ws: coworking: %v", err)
		adminBack(w, r, "", "Не удалось обновить место")
		return
	}
	if x < 1 || y < 1 || x > cw.GridCols || y > cw.GridRows {
		adminBack(w, r, "", "Координаты выходят за границы сетки коворкинга")
		return
	}
	if err := a.Workspaces.Update(r.Context(), id, name, zone, models.WorkspaceType(wtype), avail, x, y); err != nil {
		if msg := classifyWorkspaceUserError(err); msg != "" {
			adminBack(w, r, "", msg)
			return
		}
		log.Printf("admin update ws: %v", err)
		adminBack(w, r, "", "Не удалось обновить место")
		return
	}
	adminBack(w, r, "Место обновлено", "")
}

func (a *App) adminWorkspaceToggleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.FormValue("workspace_id")
	available := r.FormValue("available") == "1"
	if err := a.Workspaces.SetAvailable(r.Context(), id, available); err != nil {
		log.Printf("admin toggle ws: %v", err)
		adminBack(w, r, "", "Не удалось изменить доступность")
		return
	}
	if available {
		adminBack(w, r, "Место включено", "")
	} else {
		adminBack(w, r, "Место отключено", "")
	}
}

func (a *App) adminWorkspaceDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.FormValue("workspace_id")
	if err := a.Workspaces.Delete(r.Context(), id, time.Now()); err != nil {
		if errors.Is(err, repo.ErrWorkspaceHasBookings) {
			adminBack(w, r, "", "Нельзя удалить место с активными будущими бронированиями")
			return
		}
		log.Printf("admin delete ws: %v", err)
		adminBack(w, r, "", "Не удалось удалить место")
		return
	}
	adminBack(w, r, "Место удалено", "")
}

// --- bookings ---------------------------------------------------------------

func (a *App) adminBookingCancelHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.FormValue("booking_id")
	if err := a.Bookings.Cancel(r.Context(), id, true); err != nil {
		if errors.Is(err, repo.ErrBookingNotFound) {
			adminBack(w, r, "", "Бронирование не найдено или уже не активно")
			return
		}
		log.Printf("admin cancel: %v", err)
		adminBack(w, r, "", "Не удалось отменить бронирование")
		return
	}
	adminBack(w, r, "Бронирование отменено администратором", "")
}

// allowed admin status transitions (no rules-violating moves).
var adminStatusTransitions = map[models.BookingStatus]map[models.BookingStatus]bool{
	models.StatusConfirmed: {
		models.StatusCompleted:         true,
		models.StatusCancelledByAdmin:  true,
	},
}

func (a *App) adminBookingStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.FormValue("booking_id")
	target := models.BookingStatus(r.FormValue("status"))
	b, err := a.Bookings.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrBookingNotFound) {
			adminBack(w, r, "", "Бронирование не найдено")
			return
		}
		log.Printf("admin status: find: %v", err)
		adminBack(w, r, "", "Не удалось найти бронирование")
		return
	}
	allowed, ok := adminStatusTransitions[b.Status]
	if !ok || !allowed[target] {
		adminBack(w, r, "", "Недопустимый переход статуса")
		return
	}
	if err := a.Bookings.UpdateStatus(r.Context(), id, target); err != nil {
		log.Printf("admin status: update: %v", err)
		adminBack(w, r, "", "Не удалось изменить статус")
		return
	}
	adminBack(w, r, "Статус бронирования изменён", "")
}

// --- settings ---------------------------------------------------------------

func (a *App) adminSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, _ := auth.UserFrom(r.Context())
	v, err := strconv.Atoi(strings.TrimSpace(r.FormValue("max_active")))
	if err != nil || v <= 0 {
		adminBack(w, r, "", "Лимит должен быть положительным целым числом")
		return
	}
	if err := a.Settings.Update(r.Context(), v, user.ID); err != nil {
		log.Printf("admin settings update: %v", err)
		adminBack(w, r, "", "Не удалось сохранить настройки")
		return
	}
	adminBack(w, r, "Настройки сохранены", "")
}

// --- coworkings -------------------------------------------------------------

// classifyCoworkingUserError maps repository errors to localized admin
// messages for the coworking CRUD flows.
func classifyCoworkingUserError(err error) string {
	switch {
	case errors.Is(err, repo.ErrCoworkingNameTaken):
		return "Коворкинг с таким названием уже существует"
	case errors.Is(err, repo.ErrCoworkingNotFound):
		return "Коворкинг не найден"
	case errors.Is(err, repo.ErrCoworkingHasWorkspaces):
		return "Нельзя удалить коворкинг, в котором есть места"
	case errors.Is(err, repo.ErrCoworkingHasWorkspacesOut):
		return "Нельзя уменьшить сетку: некоторые места окажутся за её границами"
	default:
		return ""
	}
}

func parseGridSize(colsStr, rowsStr string) (cols, rows int, err error) {
	cols, err = strconv.Atoi(strings.TrimSpace(colsStr))
	if err != nil || cols < 1 || cols > repo.MaxGridDimension {
		return 0, 0, errors.New("Размер сетки должен быть от 1 до 20")
	}
	rows, err = strconv.Atoi(strings.TrimSpace(rowsStr))
	if err != nil || rows < 1 || rows > repo.MaxGridDimension {
		return 0, 0, errors.New("Размер сетки должен быть от 1 до 20")
	}
	return cols, rows, nil
}

func (a *App) adminCoworkingCreateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось обработать форму", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		adminBack(w, r, "", "Название коворкинга обязательно")
		return
	}
	cols, rows, err := parseGridSize(r.FormValue("grid_cols"), r.FormValue("grid_rows"))
	if err != nil {
		adminBack(w, r, "", err.Error())
		return
	}
	if _, err := a.Coworkings.Create(r.Context(), name, cols, rows); err != nil {
		if msg := classifyCoworkingUserError(err); msg != "" {
			adminBack(w, r, "", msg)
			return
		}
		log.Printf("admin create coworking: %v", err)
		adminBack(w, r, "", "Не удалось создать коворкинг")
		return
	}
	adminBack(w, r, "Коворкинг создан", "")
}

func (a *App) adminCoworkingUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось обработать форму", http.StatusBadRequest)
		return
	}
	id := r.FormValue("coworking_id_target")
	if id == "" {
		id = r.FormValue("coworking_id")
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if id == "" || name == "" {
		adminBack(w, r, "", "Заполните название и сетку")
		return
	}
	cols, rows, err := parseGridSize(r.FormValue("grid_cols"), r.FormValue("grid_rows"))
	if err != nil {
		adminBack(w, r, "", err.Error())
		return
	}
	if err := a.Coworkings.Update(r.Context(), id, name, cols, rows); err != nil {
		if msg := classifyCoworkingUserError(err); msg != "" {
			adminBack(w, r, "", msg)
			return
		}
		log.Printf("admin update coworking: %v", err)
		adminBack(w, r, "", "Не удалось обновить коворкинг")
		return
	}
	adminBack(w, r, "Коворкинг обновлён", "")
}

func (a *App) adminCoworkingDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Не удалось обработать форму", http.StatusBadRequest)
		return
	}
	id := r.FormValue("coworking_id_target")
	if id == "" {
		adminBack(w, r, "", "Не указан коворкинг для удаления")
		return
	}
	if err := a.Coworkings.Delete(r.Context(), id); err != nil {
		if msg := classifyCoworkingUserError(err); msg != "" {
			adminBack(w, r, "", msg)
			return
		}
		log.Printf("admin delete coworking: %v", err)
		adminBack(w, r, "", "Не удалось удалить коворкинг")
		return
	}
	adminBack(w, r, "Коворкинг удалён", "")
}
