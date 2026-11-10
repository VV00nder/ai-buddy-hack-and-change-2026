-- AI-buddy: схема хранения. Вложенные структуры — в jsonb,
-- поля для выборок и индексов — отдельными колонками.
CREATE TABLE IF NOT EXISTS step_states (
    user_id    text        NOT NULL,
    step_id    text        NOT NULL,
    status     text        NOT NULL,
    note       text        NOT NULL DEFAULT '',
    updated_by text        NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, step_id)
);

CREATE TABLE IF NOT EXISTS messages (
    id         text PRIMARY KEY,
    user_id    text        NOT NULL,
    created_at timestamptz NOT NULL,
    data       jsonb       NOT NULL
);
CREATE INDEX IF NOT EXISTS messages_user_idx ON messages (user_id, created_at);

CREATE TABLE IF NOT EXISTS actions (
    id          text PRIMARY KEY,
    user_id     text        NOT NULL,
    external_id text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL,
    data        jsonb       NOT NULL
);
CREATE INDEX IF NOT EXISTS actions_user_idx ON actions (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS actions_external_idx ON actions (external_id) WHERE external_id <> '';

CREATE TABLE IF NOT EXISTS questions (
    id         text PRIMARY KEY,
    intern_id  text        NOT NULL,
    mentor_id  text        NOT NULL,
    created_at timestamptz NOT NULL,
    data       jsonb       NOT NULL
);
CREATE INDEX IF NOT EXISTS questions_mentor_idx ON questions (mentor_id, created_at DESC);
CREATE INDEX IF NOT EXISTS questions_intern_idx ON questions (intern_id, created_at DESC);

CREATE TABLE IF NOT EXISTS notifications (
    id         text PRIMARY KEY,
    user_id    text        NOT NULL,
    read       boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    data       jsonb       NOT NULL
);
CREATE INDEX IF NOT EXISTS notifications_user_idx ON notifications (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS faq (
    id         text PRIMARY KEY,
    created_at timestamptz NOT NULL,
    data       jsonb       NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
    id      text PRIMARY KEY,
    user_id text        NOT NULL,
    type    text        NOT NULL,
    at      timestamptz NOT NULL,
    data    jsonb       NOT NULL
);
CREATE INDEX IF NOT EXISTS events_type_idx ON events (type, at);
