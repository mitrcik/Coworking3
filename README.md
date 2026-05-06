# Информационная система бронирования мест в коворкинге

Учебный прототип на **Go + PostgreSQL**, фронтенд — обычный веб-сайт, оформленный под мобильный интерфейс **Telegram Mini App** (карточки, крупные кнопки, нижняя навигация). Запускается через `docker compose`.

## Стек

- Backend: Go 1.22 (стандартная библиотека `net/http`, `html/template`)
- База данных: PostgreSQL 16
- Запуск: Docker + docker-compose
- Авторизация: email + пароль (без обязательной авторизации через Telegram)

## Запуск

```bash
docker compose up --build
```

После запуска:

- Приложение: http://localhost:8080
- PostgreSQL: `localhost:5432`, БД `coworking`, пользователь `coworking`, пароль `coworking`

Остановить и удалить тома:

```bash
docker compose down -v
```

## Учётные записи

- **Администратор:** `admin@example.com` / `admin` (создаётся автоматически на этапе 2)

## Карта этапов

- [x] Этап 1. Каркас проекта, мобильный макет всех экранов, Docker.
- [x] **Этап 2.** БД, миграции, начальные данные.
- [ ] Этап 3. Регистрация, вход, сессии, роли.
- [ ] Этап 4. Схема коворкинга и просмотр занятости мест.
- [ ] Этап 5. Создание бронирования.
- [ ] Этап 6. Мои бронирования и отмена.
- [ ] Этап 7. Админ-панель: места, бронирования, лимиты.
- [ ] Этап 8. Отчёты и финальная проверка.

## Этап 1 — что сделано

- Структура проекта Go (`cmd/server`, `internal/handlers`, `web/templates`, `web/static`).
- `Dockerfile` для Go-приложения и `docker-compose.yml` с сервисами `app` и `db` (PostgreSQL).
- HTTP-сервер с маршрутами:
  - `/` — главная,
  - `/login`, `/register` — экраны входа и регистрации,
  - `/scheme` — схема коворкинга (моковые данные),
  - `/bookings` — мои бронирования (моковые данные),
  - `/admin` — админ-панель (моковые данные).
- Мобильная вёрстка в стиле Telegram Mini App: карточки, нижняя навигация, крупные кнопки, адаптивная сетка схемы.

### Ручная проверка этапа 1

1. `docker compose up --build`
2. Открыть http://localhost:8080 — главный экран.
3. Перейти на `/login`, `/register`, `/scheme`, `/bookings`, `/admin` и убедиться, что отображаются все экраны.
4. Открыть DevTools, включить мобильный просмотр (например, iPhone 12 Pro) и проверить, что вёрстка адаптивная.
5. Проверить, что нижняя навигация переключает экраны.
6. PostgreSQL поднимается без ошибок (на этапе 2 будут созданы таблицы).

### Известные ограничения этапа 1

- Все данные — моковые, БД пока не используется приложением.
- Формы входа и регистрации не обрабатываются на сервере — это будет реализовано на этапе 3.
- Бронирование, отмена и админские функции — заглушки в виде интерфейса.

## Этап 2 — что сделано

- БД PostgreSQL автоматически инициализируется скриптами `db/init/01_schema.sql` (схема) и `db/init/02_seed.sql` (начальные данные) через `/docker-entrypoint-initdb.d/`.
- Созданы все 6 сущностей из `.md`-файла: `users`, `workspaces`, `bookings`, `booking_settings`, `reports`, `notifications`.
- Созданы enum-типы: `role`, `workspace_type`, `booking_status`, `notification_type`.
- Внешние ключи: `bookings.user_id` → `users`, `bookings.workspace_id` → `workspaces`, `notifications.user_id` → `users`, `notifications.booking_id` → `bookings`, `reports.created_by` → `users`, `booking_settings.updated_by` → `users`.
- Ограничения: уникальный `email` пользователя, уникальное `name` места, `bookings.end_time > start_time`, `max_active_bookings_per_user > 0`, singleton-индекс на `booking_settings`.
- Начальные данные: администратор `admin@example.com` / `admin` (хеш bcrypt через `pgcrypto.crypt(... 'bf')`), 9 рабочих мест на сетке 3×3, настройки с лимитом 3 активных бронирования.
- Go-приложение подключается к PostgreSQL (драйвер `lib/pq`) и на странице `/scheme` загружает места из БД.

### Соответствие сущностей `.md`-файла и таблиц

| Сущность из `.md` | Таблица      | Поля из `.md` → колонки |
|---|---|---|
| `User`            | `users`            | `userId` → `user_id`, `fullName` → `full_name`, `email`, `passwordHash` → `password_hash`, `role`, `activeBookingCount` → `active_booking_count`, `createdAt` → `created_at` |
| `Workspace`       | `workspaces`       | `workspaceId` → `workspace_id`, `name`, `type`, `isAvailable` → `is_available`, `positionX`/`positionY` → `position_x`/`position_y`, `createdAt` → `created_at`, плюс техническое `zone` для группировки |
| `Booking`         | `bookings`         | `bookingId` → `booking_id`, `startTime` → `start_time`, `endTime` → `end_time`, `status`, `createdAt` → `created_at`, `cancelledAt` → `cancelled_at`, плюс `user_id`, `workspace_id` для связей |
| `BookingSettings` | `booking_settings` | `settingsId` → `settings_id`, `maxActiveBookingsPerUser` → `max_active_bookings_per_user`, `cancellationLeadTimeHours` → `cancellation_lead_time_hours`, `updatedBy` → `updated_by` (FK users), `updatedAt` → `updated_at` |
| `Report`          | `reports`          | `reportId` → `report_id`, `generatedAt` → `generated_at`, `dateRangeStart`/`dateRangeEnd` → `date_range_start`/`date_range_end`, `workspaceTypeFilter` → `workspace_type_filter`, `data` (JSONB), плюс `created_by` (FK users) |
| `Notification`    | `notifications`    | `notificationId` → `notification_id`, `type`, `message`, `sentAt` → `sent_at`, `isRead` → `is_read`, плюс `user_id`, `booking_id` для связей |

### Ручная проверка этапа 2

```bash
docker compose down -v
docker compose up --build
```

1. Проверить логи приложения: должно быть `connected to postgres db:5432/coworking` и `server listening on :8080`.
2. Список таблиц:
   ```bash
   docker exec coworking_db psql -U coworking -d coworking -c "\dt"
   ```
3. Список enum-типов:
   ```bash
   docker exec coworking_db psql -U coworking -d coworking -c "\dT+"
   ```
4. Администратор существует, пароль захеширован:
   ```bash
   docker exec coworking_db psql -U coworking -d coworking \
     -c "SELECT email, role, LEFT(password_hash, 4) AS hash_prefix, LENGTH(password_hash) FROM users;"
   ```
   Должно быть: `admin@example.com`, `ADMIN`, `$2a$`, 60.
5. На `/scheme` отображаются места из БД (A1–A3, B1–B3, M1–M2, L1).

### Известные ограничения этапа 2

- Авторизация ещё не реализована (этап 3) — кнопка «Войти» пока не выполняет вход.
- Список «Мои бронирования» по-прежнему использует моковые данные.

## Структура проекта

```
.
├── cmd/server/main.go           # entry point
├── internal/handlers/handlers.go # HTTP handlers
├── web/templates/                # HTML-шаблоны
└── web/static/css/style.css      # мобильные стили
```
