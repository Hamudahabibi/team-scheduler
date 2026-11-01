-- +goose Up

-- Команды, для которых запускалась генерация. При публикации их прежние
-- тренировки на неделю отменяются, даже если новых им не досталось.
ALTER TABLE schedule_runs ADD COLUMN team_ids BIGINT[] NOT NULL DEFAULT '{}';

-- Прогноз посещаемости на момент планирования (доля взвешенного состава).
-- Нужен, чтобы сравнивать прогноз с фактической посещаемостью.
ALTER TABLE training_sessions ADD COLUMN expected_attendance DOUBLE PRECISION NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE training_sessions DROP COLUMN expected_attendance;
ALTER TABLE schedule_runs DROP COLUMN team_ids;
