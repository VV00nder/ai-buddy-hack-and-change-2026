-- Цели стажировки (трекер прогресса из чек-листа Авито).
CREATE TABLE IF NOT EXISTS goals (
    id         text PRIMARY KEY,
    intern_id  text        NOT NULL,
    created_at timestamptz NOT NULL,
    data       jsonb       NOT NULL
);
CREATE INDEX IF NOT EXISTS goals_intern_idx ON goals (intern_id, created_at);
