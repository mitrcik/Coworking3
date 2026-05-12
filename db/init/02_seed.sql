-- Начальные данные (Stage 2). Скрипт идемпотентный.

-- 1) Администратор admin@example.com / admin.
-- pgcrypto crypt() с алгоритмом 'bf' даёт bcrypt-совместимый хеш ($2a$),
-- который проверяется библиотекой golang.org/x/crypto/bcrypt.
INSERT INTO users (full_name, email, password_hash, role)
SELECT 'Администратор', 'admin@example.com', crypt('admin', gen_salt('bf', 10)), 'ADMIN'
WHERE NOT EXISTS (SELECT 1 FROM users WHERE email = 'admin@example.com');

-- 2) Дефолтные настройки (singleton).
INSERT INTO booking_settings (max_active_bookings_per_user, cancellation_lead_time_hours, updated_by)
SELECT 3, 1, (SELECT user_id FROM users WHERE email = 'admin@example.com')
WHERE NOT EXISTS (SELECT 1 FROM booking_settings);

-- 3) Начальные рабочие места для схемы коворкинга.
--    Сетка 3x3 — три зоны: тихая (Y=1), командная (Y=2), переговорные/лаунж (Y=3).
INSERT INTO workspaces (name, type, zone, is_available, position_x, position_y)
SELECT v.name, v.wtype::workspace_type, v.zone, v.is_available, v.x, v.y
FROM (
    VALUES
        ('A1', 'DESK',         'Тихая зона',   TRUE,  1, 1),
        ('A2', 'DESK',         'Тихая зона',   TRUE,  2, 1),
        ('A3', 'DESK',         'Тихая зона',   TRUE,  3, 1),
        ('B1', 'DESK',         'Командная',    TRUE,  1, 2),
        ('B2', 'DESK',         'Командная',    FALSE, 2, 2), -- временно отключено
        ('B3', 'DESK',         'Командная',    TRUE,  3, 2),
        ('M1', 'MEETING_ROOM', 'Переговорные', TRUE,  1, 3),
        ('M2', 'MEETING_ROOM', 'Переговорные', TRUE,  2, 3),
        ('L1', 'LOUNGE',       'Лаунж',        TRUE,  3, 3)
) AS v(name, wtype, zone, is_available, x, y)
WHERE NOT EXISTS (SELECT 1 FROM workspaces WHERE workspaces.name = v.name);
