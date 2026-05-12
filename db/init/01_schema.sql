-- Coworking booking system schema (Stage 2).
-- Соответствие .md-файлу:
--   User             -> users
--   Workspace        -> workspaces
--   Booking          -> bookings
--   BookingSettings  -> booking_settings
--   Report           -> reports
--   Notification     -> notifications

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ENUM types из .md ----------------------------------------------------------

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'role') THEN
        CREATE TYPE role AS ENUM ('USER', 'ADMIN');
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'workspace_type') THEN
        CREATE TYPE workspace_type AS ENUM ('DESK', 'MEETING_ROOM', 'LOUNGE');
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'booking_status') THEN
        CREATE TYPE booking_status AS ENUM (
            'CONFIRMED',
            'COMPLETED',
            'CANCELLED_BY_USER',
            'CANCELLED_BY_ADMIN'
        );
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'notification_type') THEN
        CREATE TYPE notification_type AS ENUM (
            'BOOKING_CONFIRMED',
            'BOOKING_CANCELLED',
            'REMINDER'
        );
    END IF;
END $$;

-- USER -----------------------------------------------------------------------
-- userId, fullName, email, passwordHash, role, activeBookingCount, createdAt
CREATE TABLE IF NOT EXISTS users (
    user_id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name            TEXT        NOT NULL,
    email                TEXT        NOT NULL UNIQUE,
    password_hash        TEXT        NOT NULL,
    role                 role        NOT NULL DEFAULT 'USER',
    active_booking_count INTEGER     NOT NULL DEFAULT 0
                         CHECK (active_booking_count >= 0),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- WORKSPACE ------------------------------------------------------------------
-- workspaceId, name, type, isAvailable, positionX, positionY, createdAt
-- Поле zone добавлено как "техническое" поле для группировки мест по зонам
-- (упоминается в UC-2/UC-5 как параметр места).
CREATE TABLE IF NOT EXISTS workspaces (
    workspace_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT           NOT NULL UNIQUE,
    type          workspace_type NOT NULL,
    zone          TEXT           NOT NULL DEFAULT '',
    is_available  BOOLEAN        NOT NULL DEFAULT TRUE,
    position_x    INTEGER        NOT NULL,
    position_y    INTEGER        NOT NULL,
    created_at    TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

-- BOOKING --------------------------------------------------------------------
-- bookingId, startTime, endTime, status, createdAt, cancelledAt
-- + связи Booking ↔ User и Booking ↔ Workspace (FK)
CREATE TABLE IF NOT EXISTS bookings (
    booking_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID           NOT NULL REFERENCES users(user_id)      ON DELETE CASCADE,
    workspace_id  UUID           NOT NULL REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    start_time    TIMESTAMPTZ    NOT NULL,
    end_time      TIMESTAMPTZ    NOT NULL,
    status        booking_status NOT NULL DEFAULT 'CONFIRMED',
    created_at    TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    cancelled_at  TIMESTAMPTZ    NULL,
    CONSTRAINT bookings_time_order CHECK (end_time > start_time)
);

CREATE INDEX IF NOT EXISTS bookings_user_id_idx       ON bookings (user_id);
CREATE INDEX IF NOT EXISTS bookings_workspace_id_idx  ON bookings (workspace_id);
CREATE INDEX IF NOT EXISTS bookings_time_idx          ON bookings (start_time, end_time);

-- BOOKING_SETTINGS -----------------------------------------------------------
-- settingsId (singleton), maxActiveBookingsPerUser, cancellationLeadTimeHours,
-- updatedBy, updatedAt
-- Связь User ↔ BookingSettings: updated_by ссылается на users.
CREATE TABLE IF NOT EXISTS booking_settings (
    settings_id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    max_active_bookings_per_user   INTEGER NOT NULL CHECK (max_active_bookings_per_user > 0),
    cancellation_lead_time_hours   INTEGER NOT NULL DEFAULT 1
                                   CHECK (cancellation_lead_time_hours >= 0),
    updated_by                     UUID NULL REFERENCES users(user_id) ON DELETE SET NULL,
    updated_at                     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Singleton-флаг: разрешаем только одну строку.
CREATE UNIQUE INDEX IF NOT EXISTS booking_settings_singleton ON booking_settings ((TRUE));

-- REPORT ---------------------------------------------------------------------
-- reportId, generatedAt, dateRangeStart, dateRangeEnd, workspaceTypeFilter,
-- data (JSON). Связь User ↔ Report: created_by.
CREATE TABLE IF NOT EXISTS reports (
    report_id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    generated_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    date_range_start       TIMESTAMPTZ    NOT NULL,
    date_range_end         TIMESTAMPTZ    NOT NULL,
    workspace_type_filter  workspace_type NULL,
    data                   JSONB          NOT NULL DEFAULT '{}'::jsonb,
    created_by             UUID           NULL REFERENCES users(user_id) ON DELETE SET NULL,
    CONSTRAINT reports_range CHECK (date_range_end >= date_range_start)
);

-- NOTIFICATION ---------------------------------------------------------------
-- notificationId, type, message, sentAt, isRead. Связи: User и Booking.
CREATE TABLE IF NOT EXISTS notifications (
    notification_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID              NOT NULL REFERENCES users(user_id)    ON DELETE CASCADE,
    booking_id       UUID              NULL    REFERENCES bookings(booking_id) ON DELETE CASCADE,
    type             notification_type NOT NULL,
    message          TEXT              NOT NULL,
    sent_at          TIMESTAMPTZ       NOT NULL DEFAULT NOW(),
    is_read          BOOLEAN           NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS notifications_user_id_idx    ON notifications (user_id);
CREATE INDEX IF NOT EXISTS notifications_booking_id_idx ON notifications (booking_id);
