-- +goose Up

CREATE TABLE study_groups (
    id   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code TEXT NOT NULL UNIQUE CHECK (code <> '')
);

CREATE TABLE users (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email          TEXT NOT NULL,
    password_hash  TEXT NOT NULL,
    full_name      TEXT NOT NULL CHECK (full_name <> ''),
    role           TEXT NOT NULL DEFAULT 'player' CHECK (role IN ('admin', 'coach', 'player')),
    study_group_id BIGINT REFERENCES study_groups (id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- E-mail уникален без учёта регистра.
CREATE UNIQUE INDEX users_email_uniq ON users (lower(email));

CREATE TABLE auth_tokens (
    token_hash BYTEA PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX auth_tokens_user_idx ON auth_tokens (user_id);

CREATE TABLE class_events (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    group_id  BIGINT NOT NULL REFERENCES study_groups (id) ON DELETE CASCADE,
    title     TEXT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at   TIMESTAMPTZ NOT NULL,
    CHECK (ends_at > starts_at)
);
CREATE INDEX class_events_group_time_idx ON class_events (group_id, starts_at);

CREATE TABLE disciplines (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    roster_size INT  NOT NULL CHECK (roster_size BETWEEN 1 AND 20)
);

INSERT INTO disciplines (code, name, roster_size) VALUES
    ('mlbb', 'Mobile Legends: Bang Bang', 5),
    ('cs2', 'Counter-Strike 2', 5),
    ('dota2', 'Dota 2', 5),
    ('valorant', 'Valorant', 5),
    ('lol', 'League of Legends', 5);

CREATE TABLE teams (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE CHECK (name <> ''),
    discipline_id BIGINT NOT NULL REFERENCES disciplines (id),
    coach_id      BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE team_members (
    team_id   BIGINT NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    user_id   BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role      TEXT NOT NULL CHECK (role IN ('core', 'substitute')),
    position  TEXT NOT NULL DEFAULT '',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_id)
);
CREATE INDEX team_members_user_idx ON team_members (user_id);

-- Тренировочный план команды на микроцикл (неделю). Время — минуты от полуночи.
CREATE TABLE training_plans (
    team_id           BIGINT PRIMARY KEY REFERENCES teams (id) ON DELETE CASCADE,
    sessions_per_week INT NOT NULL CHECK (sessions_per_week BETWEEN 1 AND 7),
    duration_min      INT NOT NULL CHECK (duration_min BETWEEN 30 AND 480 AND duration_min % 30 = 0),
    min_core_players  INT NOT NULL CHECK (min_core_players >= 1),
    day_start_min     INT NOT NULL CHECK (day_start_min BETWEEN 0 AND 1440),
    day_end_min       INT NOT NULL CHECK (day_end_min BETWEEN 0 AND 1440),
    min_rest_min      INT NOT NULL DEFAULT 0 CHECK (min_rest_min >= 0),
    priority          INT NOT NULL DEFAULT 1 CHECK (priority BETWEEN 1 AND 10),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (day_end_min > day_start_min)
);

-- Еженедельные правила доступности игрока. weekday: 1 = понедельник … 7 = воскресенье.
CREATE TABLE availability_rules (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id   BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    weekday   INT  NOT NULL CHECK (weekday BETWEEN 1 AND 7),
    start_min INT  NOT NULL CHECK (start_min BETWEEN 0 AND 1440),
    end_min   INT  NOT NULL CHECK (end_min BETWEEN 0 AND 1440),
    kind      TEXT NOT NULL CHECK (kind IN ('available', 'preferred')),
    CHECK (end_min > start_min)
);
CREATE INDEX availability_rules_user_idx ON availability_rules (user_id);

CREATE TABLE availability_exceptions (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id   BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at   TIMESTAMPTZ NOT NULL,
    kind      TEXT NOT NULL CHECK (kind IN ('unavailable', 'available')),
    note      TEXT NOT NULL DEFAULT '',
    CHECK (ends_at > starts_at)
);
CREATE INDEX availability_exceptions_user_time_idx ON availability_exceptions (user_id, starts_at);

CREATE TABLE rooms (
    id    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name  TEXT NOT NULL UNIQUE CHECK (name <> ''),
    seats INT  NOT NULL CHECK (seats > 0)
);

CREATE TABLE room_hours (
    room_id   BIGINT NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    weekday   INT NOT NULL CHECK (weekday BETWEEN 1 AND 7),
    open_min  INT NOT NULL CHECK (open_min BETWEEN 0 AND 1440),
    close_min INT NOT NULL CHECK (close_min BETWEEN 0 AND 1440),
    PRIMARY KEY (room_id, weekday),
    CHECK (close_min > open_min)
);

CREATE TABLE room_blocks (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id   BIGINT NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at   TIMESTAMPTZ NOT NULL,
    reason    TEXT NOT NULL DEFAULT '',
    CHECK (ends_at > starts_at)
);
CREATE INDEX room_blocks_room_time_idx ON room_blocks (room_id, starts_at);

CREATE TABLE schedule_runs (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    week_start   DATE NOT NULL,
    algorithm    TEXT NOT NULL CHECK (algorithm IN ('greedy', 'bnb')),
    status       TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'discarded')),
    objective    DOUBLE PRECISION NOT NULL DEFAULT 0,
    stats        JSONB NOT NULL DEFAULT '{}',
    created_by   BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    -- Неделя всегда начинается с понедельника (ISODOW = 1).
    CHECK (extract(isodow FROM week_start) = 1)
);
CREATE INDEX schedule_runs_week_idx ON schedule_runs (week_start);

CREATE TABLE training_sessions (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id     BIGINT REFERENCES schedule_runs (id) ON DELETE CASCADE,
    team_id    BIGINT NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    room_id    BIGINT NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    starts_at  TIMESTAMPTZ NOT NULL,
    ends_at    TIMESTAMPTZ NOT NULL,
    seats      INT NOT NULL CHECK (seats > 0),
    status     TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'cancelled')),
    quality    DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at)
);
CREATE INDEX training_sessions_team_time_idx ON training_sessions (team_id, starts_at);
CREATE INDEX training_sessions_room_time_idx ON training_sessions (room_id, starts_at) WHERE status = 'published';
CREATE INDEX training_sessions_run_idx ON training_sessions (run_id);

CREATE TABLE attendance (
    session_id BIGINT NOT NULL REFERENCES training_sessions (id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status     TEXT NOT NULL CHECK (status IN ('present', 'late', 'absent', 'excused')),
    marked_by  BIGINT REFERENCES users (id) ON DELETE SET NULL,
    marked_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (session_id, user_id)
);

-- +goose Down

DROP TABLE attendance;
DROP TABLE training_sessions;
DROP TABLE schedule_runs;
DROP TABLE room_blocks;
DROP TABLE room_hours;
DROP TABLE rooms;
DROP TABLE availability_exceptions;
DROP TABLE availability_rules;
DROP TABLE training_plans;
DROP TABLE team_members;
DROP TABLE teams;
DROP TABLE disciplines;
DROP TABLE class_events;
DROP TABLE auth_tokens;
DROP TABLE users;
DROP TABLE study_groups;
