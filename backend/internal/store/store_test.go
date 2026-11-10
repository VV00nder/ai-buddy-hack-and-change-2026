package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"aibuddy/internal/domain"
)

func contract(t *testing.T, s Store) {
	ctx := context.Background()
	t0 := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	if err := s.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	// шаги плана
	if st, _ := s.StepStates(ctx, "u1"); len(st) != 0 {
		t.Fatal("empty")
	}
	_ = s.SetStepState(ctx, domain.StepState{UserID: "u1", StepID: "b", Status: domain.StepTodo, UpdatedAt: t0})
	_ = s.SetStepState(ctx, domain.StepState{UserID: "u1", StepID: "a", Status: domain.StepDone, Note: "n", UpdatedBy: "u1", UpdatedAt: t0})
	_ = s.SetStepState(ctx, domain.StepState{UserID: "u1", StepID: "b", Status: domain.StepInProgress, UpdatedAt: t0})
	st, err := s.StepStates(ctx, "u1")
	if err != nil || len(st) != 2 || st[0].StepID != "a" || st[1].Status != domain.StepInProgress || st[0].Note != "n" {
		t.Fatalf("steps: %+v %v", st, err)
	}
	// сообщения
	for i, id := range []string{"m1", "m2", "m3"} {
		m := domain.Message{ID: id, UserID: "u1", Author: "user", Text: id, Status: domain.MsgDone, CreatedAt: t0.Add(time.Duration(i) * time.Second)}
		if err := s.SaveMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.SaveMessage(ctx, domain.Message{ID: "x", UserID: "u2", CreatedAt: t0})
	m2, _ := s.Message(ctx, "m2")
	m2.Text, m2.Sources = "updated", []domain.Source{{DocID: "d", Title: "D"}}
	_ = s.SaveMessage(ctx, m2)
	msgs, err := s.Messages(ctx, "u1", 2)
	if err != nil || len(msgs) != 2 || msgs[0].ID != "m2" || msgs[0].Text != "updated" || msgs[0].Sources[0].DocID != "d" || msgs[1].ID != "m3" {
		t.Fatalf("messages: %+v %v", msgs, err)
	}
	if all, _ := s.Messages(ctx, "u1", 0); len(all) != 3 {
		t.Fatal("all messages")
	}
	if _, err := s.Message(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatal("msg not found")
	}
	// действия
	a := domain.Action{ID: "a1", UserID: "u1", Kind: domain.ActionAbsence, Status: domain.ActionProposed, Args: map[string]any{"date_from": "2026-11-13"}, CreatedAt: t0}
	_ = s.SaveAction(ctx, a)
	_ = s.SaveAction(ctx, domain.Action{ID: "a2", UserID: "u1", Kind: domain.ActionAccess, CreatedAt: t0.Add(time.Minute)})
	a.Status, a.ExternalID = domain.ActionSubmitted, "ABS-1"
	_ = s.SaveAction(ctx, a)
	got, err := s.ActionByExternalID(ctx, "ABS-1")
	if err != nil || got.ID != "a1" || got.Args["date_from"] != "2026-11-13" {
		t.Fatalf("by external: %+v %v", got, err)
	}
	if _, err := s.ActionByExternalID(ctx, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("empty external")
	}
	if g, _ := s.Action(ctx, "a1"); g.Status != domain.ActionSubmitted {
		t.Fatal("action")
	}
	if _, err := s.Action(ctx, "none"); !errors.Is(err, ErrNotFound) {
		t.Fatal("action not found")
	}
	if l, _ := s.Actions(ctx, "u1"); len(l) != 2 || l[0].ID != "a2" {
		t.Fatalf("actions: %+v", l)
	}
	// вопросы
	q := domain.Question{ID: "q1", InternID: "u1", MentorID: "m", Text: "?", Status: "open", CreatedAt: t0}
	_ = s.SaveQuestion(ctx, q)
	q.Status, q.Answer = "answered", "!"
	_ = s.SaveQuestion(ctx, q)
	if g, _ := s.Question(ctx, "q1"); g.Answer != "!" {
		t.Fatal("question")
	}
	if _, err := s.Question(ctx, "none"); !errors.Is(err, ErrNotFound) {
		t.Fatal("question not found")
	}
	if l, _ := s.QuestionsForMentor(ctx, "m"); len(l) != 1 {
		t.Fatal("for mentor")
	}
	if l, _ := s.QuestionsOfIntern(ctx, "u1"); len(l) != 1 {
		t.Fatal("of intern")
	}
	// уведомления
	for i := 0; i < 3; i++ {
		_ = s.AddNotification(ctx, domain.Notification{ID: string(rune('a' + i)), UserID: "u1", Text: "t", CreatedAt: t0.Add(time.Duration(i) * time.Second)})
	}
	n, _ := s.Notifications(ctx, "u1", 2)
	if len(n) != 2 || n[0].ID != "c" || n[0].Read {
		t.Fatalf("notifications: %+v", n)
	}
	_ = s.MarkNotificationsRead(ctx, "u1")
	if n, _ := s.Notifications(ctx, "u1", 0); len(n) != 3 || !n[2].Read {
		t.Fatalf("read: %+v", n)
	}
	// FAQ и события
	_ = s.AddFAQ(ctx, domain.FAQEntry{ID: "f1", Team: "search", Question: "q", Answer: "a", CreatedAt: t0})
	if f, _ := s.FAQs(ctx); len(f) != 1 || f[0].Team != "search" {
		t.Fatal("faq")
	}
	_ = s.AddEvent(ctx, domain.Event{ID: "e1", UserID: "u1", Type: "question_asked", Props: map[string]any{"k": "v"}, At: t0})
	if e, _ := s.Events(ctx); len(e) != 1 || e[0].Props["k"] != "v" {
		t.Fatal("events")
	}
}

func TestMemory(t *testing.T) { contract(t, NewMemory()) }

func TestPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан — тест PostgreSQL пропущен")
	}
	ctx := context.Background()
	p, err := OpenPostgres(ctx, dsn, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.Truncate(ctx); err != nil {
		t.Fatal(err)
	}
	contract(t, p)
	// повторное применение миграций идемпотентно
	p2, err := OpenPostgres(ctx, dsn, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = p2.Close()
	// ошибки после закрытия соединения
	_ = p.Close()
	if _, err := p.StepStates(ctx, "u1"); err == nil {
		t.Fatal("closed db")
	}
	if _, err := p.Messages(ctx, "u1", 0); err == nil {
		t.Fatal("closed db")
	}
	if _, err := p.Message(ctx, "x"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatal("closed db get")
	}
	if err := p.SaveMessage(ctx, domain.Message{Trace: []domain.TraceItem{}, Sources: nil, Feedback: nil, Suggestions: []string{}}); err == nil {
		t.Fatal("closed db save")
	}
}

func TestPostgresUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := OpenPostgres(ctx, "postgres://x:y@127.0.0.1:1/db?sslmode=disable&connect_timeout=1", 600*time.Millisecond); err == nil {
		t.Fatal("must fail")
	}
	if _, err := OpenPostgres(ctx, "://bad", 0); err == nil {
		t.Fatal("bad dsn")
	}
}
