package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq" // драйвер PostgreSQL

	"aibuddy/internal/domain"
)

//go:embed migrations/001_init.sql
var migration001 string

//go:embed migrations/002_goals.sql
var migration002 string

//go:embed migrations/003_docs.sql
var migration003 string

// Postgres — хранилище в PostgreSQL.
type Postgres struct{ db *sql.DB }

// OpenPostgres подключается, ждёт готовности БД и применяет миграции.
func OpenPostgres(ctx context.Context, dsn string, wait time.Duration) (*Postgres, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	deadline := time.Now().Add(wait)
	for {
		err = db.PingContext(ctx)
		if err == nil || time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres: %w", err)
	}
	if _, err := db.ExecContext(ctx, migration001+"\n"+migration002+"\n"+migration003); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres migrate: %w", err)
	}
	return &Postgres{db: db}, nil
}

// Close закрывает соединения.
func (p *Postgres) Close() error { return p.db.Close() }

// Truncate очищает таблицы (для тестов и сброса демо).
func (p *Postgres) Truncate(ctx context.Context) error {
	_, err := p.db.ExecContext(ctx, `TRUNCATE step_states, messages, actions, questions, notifications, faq, events, goals, records`)
	return err
}

func (p *Postgres) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }

func (p *Postgres) StepStates(ctx context.Context, userID string) ([]domain.StepState, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT user_id, step_id, status, note, updated_by, updated_at FROM step_states WHERE user_id=$1 ORDER BY step_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.StepState{}
	for rows.Next() {
		var s domain.StepState
		if err := rows.Scan(&s.UserID, &s.StepID, &s.Status, &s.Note, &s.UpdatedBy, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (p *Postgres) SetStepState(ctx context.Context, s domain.StepState) error {
	_, err := p.db.ExecContext(ctx, `INSERT INTO step_states (user_id, step_id, status, note, updated_by, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (user_id, step_id) DO UPDATE SET status=EXCLUDED.status, note=EXCLUDED.note,
		updated_by=EXCLUDED.updated_by, updated_at=EXCLUDED.updated_at`,
		s.UserID, s.StepID, s.Status, s.Note, s.UpdatedBy, s.UpdatedAt)
	return err
}

func (p *Postgres) upsert(ctx context.Context, query string, v any, args ...any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, query, append(args, data)...)
	return err
}

func getOne[T any](ctx context.Context, db *sql.DB, query string, args ...any) (T, error) {
	var v T
	var data []byte
	err := db.QueryRowContext(ctx, query, args...).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	err = json.Unmarshal(data, &v)
	return v, err
}

func getMany[T any](ctx context.Context, db *sql.DB, query string, args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var v T
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (p *Postgres) SaveMessage(ctx context.Context, m domain.Message) error {
	return p.upsert(ctx, `INSERT INTO messages (id, user_id, created_at, data) VALUES ($1,$2,$3,$4)
		ON CONFLICT (id) DO UPDATE SET data=EXCLUDED.data`, m, m.ID, m.UserID, m.CreatedAt)
}

func (p *Postgres) Message(ctx context.Context, id string) (domain.Message, error) {
	return getOne[domain.Message](ctx, p.db, `SELECT data FROM messages WHERE id=$1`, id)
}

func (p *Postgres) Messages(ctx context.Context, userID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 1000
	}
	return getMany[domain.Message](ctx, p.db, `SELECT data FROM (SELECT data, created_at, id FROM messages WHERE user_id=$1
		ORDER BY created_at DESC, id DESC LIMIT $2) t ORDER BY created_at, id`, userID, limit)
}

func (p *Postgres) SaveAction(ctx context.Context, a domain.Action) error {
	return p.upsert(ctx, `INSERT INTO actions (id, user_id, external_id, created_at, data) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (id) DO UPDATE SET external_id=EXCLUDED.external_id, data=EXCLUDED.data`, a, a.ID, a.UserID, a.ExternalID, a.CreatedAt)
}

func (p *Postgres) Action(ctx context.Context, id string) (domain.Action, error) {
	return getOne[domain.Action](ctx, p.db, `SELECT data FROM actions WHERE id=$1`, id)
}

func (p *Postgres) ActionByExternalID(ctx context.Context, ext string) (domain.Action, error) {
	return getOne[domain.Action](ctx, p.db, `SELECT data FROM actions WHERE external_id=$1 AND external_id<>'' ORDER BY created_at DESC LIMIT 1`, ext)
}

func (p *Postgres) Actions(ctx context.Context, userID string) ([]domain.Action, error) {
	return getMany[domain.Action](ctx, p.db, `SELECT data FROM actions WHERE user_id=$1 ORDER BY created_at DESC`, userID)
}

func (p *Postgres) SaveQuestion(ctx context.Context, q domain.Question) error {
	return p.upsert(ctx, `INSERT INTO questions (id, intern_id, mentor_id, created_at, data) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (id) DO UPDATE SET data=EXCLUDED.data`, q, q.ID, q.InternID, q.MentorID, q.CreatedAt)
}

func (p *Postgres) Question(ctx context.Context, id string) (domain.Question, error) {
	return getOne[domain.Question](ctx, p.db, `SELECT data FROM questions WHERE id=$1`, id)
}

func (p *Postgres) QuestionsForMentor(ctx context.Context, id string) ([]domain.Question, error) {
	return getMany[domain.Question](ctx, p.db, `SELECT data FROM questions WHERE mentor_id=$1 ORDER BY created_at DESC`, id)
}

func (p *Postgres) QuestionsOfIntern(ctx context.Context, id string) ([]domain.Question, error) {
	return getMany[domain.Question](ctx, p.db, `SELECT data FROM questions WHERE intern_id=$1 ORDER BY created_at DESC`, id)
}

func (p *Postgres) AddNotification(ctx context.Context, n domain.Notification) error {
	return p.upsert(ctx, `INSERT INTO notifications (id, user_id, read, created_at, data) VALUES ($1,$2,$3,$4,$5)`,
		n, n.ID, n.UserID, n.Read, n.CreatedAt)
}

func (p *Postgres) Notifications(ctx context.Context, userID string, limit int) ([]domain.Notification, error) {
	if limit <= 0 {
		limit = 1000
	}
	return getMany[domain.Notification](ctx, p.db, `SELECT jsonb_set(data, '{read}', to_jsonb(read)) FROM notifications
		WHERE user_id=$1 ORDER BY created_at DESC, id DESC LIMIT $2`, userID, limit)
}

func (p *Postgres) MarkNotificationsRead(ctx context.Context, userID string) error {
	_, err := p.db.ExecContext(ctx, `UPDATE notifications SET read=true WHERE user_id=$1`, userID)
	return err
}

func (p *Postgres) AddFAQ(ctx context.Context, f domain.FAQEntry) error {
	return p.upsert(ctx, `INSERT INTO faq (id, created_at, data) VALUES ($1,$2,$3)`, f, f.ID, f.CreatedAt)
}

func (p *Postgres) FAQs(ctx context.Context) ([]domain.FAQEntry, error) {
	return getMany[domain.FAQEntry](ctx, p.db, `SELECT data FROM faq ORDER BY created_at`)
}

func (p *Postgres) SaveGoal(ctx context.Context, g domain.Goal) error {
	return p.upsert(ctx, `INSERT INTO goals (id, intern_id, created_at, data) VALUES ($1,$2,$3,$4)
		ON CONFLICT (id) DO UPDATE SET data=EXCLUDED.data`, g, g.ID, g.InternID, g.CreatedAt)
}

func (p *Postgres) Goal(ctx context.Context, id string) (domain.Goal, error) {
	return getOne[domain.Goal](ctx, p.db, `SELECT data FROM goals WHERE id=$1`, id)
}

func (p *Postgres) Goals(ctx context.Context, internID string) ([]domain.Goal, error) {
	return getMany[domain.Goal](ctx, p.db, `SELECT data FROM goals WHERE intern_id=$1 ORDER BY created_at, id`, internID)
}

func (p *Postgres) DeleteGoal(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM goals WHERE id=$1`, id)
	return err
}

func (p *Postgres) AddEvent(ctx context.Context, e domain.Event) error {
	return p.upsert(ctx, `INSERT INTO events (id, user_id, type, at, data) VALUES ($1,$2,$3,$4,$5)`, e, e.ID, e.UserID, e.Type, e.At)
}

func (p *Postgres) Events(ctx context.Context) ([]domain.Event, error) {
	return getMany[domain.Event](ctx, p.db, `SELECT data FROM events ORDER BY at, id`)
}

func (p *Postgres) PutRecord(ctx context.Context, kind, id string, data []byte) error {
	_, err := p.db.ExecContext(ctx, `INSERT INTO records (kind, id, updated_at, data) VALUES ($1,$2,now(),$3)
		ON CONFLICT (kind, id) DO UPDATE SET data=EXCLUDED.data, updated_at=now()`, kind, id, data)
	return err
}

func (p *Postgres) Record(ctx context.Context, kind, id string) ([]byte, error) {
	var b []byte
	err := p.db.QueryRowContext(ctx, `SELECT data FROM records WHERE kind=$1 AND id=$2`, kind, id).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return b, err
}

func (p *Postgres) Records(ctx context.Context, kind string) ([][]byte, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT data FROM records WHERE kind=$1 ORDER BY id`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := [][]byte{}
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (p *Postgres) DeleteRecord(ctx context.Context, kind, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM records WHERE kind=$1 AND id=$2`, kind, id)
	return err
}
