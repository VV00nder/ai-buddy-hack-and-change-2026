-- Записи, редактируемые в интерфейсе: инструкции, папки, тесты и их
-- прохождения, шаблоны обучения, программы стажёров, выданные права.
CREATE TABLE IF NOT EXISTS records (
    kind       text        NOT NULL,
    id         text        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    data       jsonb       NOT NULL,
    PRIMARY KEY (kind, id)
);
