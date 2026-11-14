// Package store — хранилище AI-buddy: PostgreSQL в рабочем режиме и
// in-memory реализация для локальных проверок и тестов.
package store

import (
	"context"
	"errors"

	"aibuddy/internal/domain"
)

// ErrNotFound — запись не найдена.
var ErrNotFound = errors.New("not found")

// Store — операции хранения.
type Store interface {
	Ping(ctx context.Context) error

	StepStates(ctx context.Context, userID string) ([]domain.StepState, error)
	SetStepState(ctx context.Context, s domain.StepState) error

	SaveMessage(ctx context.Context, m domain.Message) error
	Message(ctx context.Context, id string) (domain.Message, error)
	Messages(ctx context.Context, userID string, limit int) ([]domain.Message, error)

	SaveAction(ctx context.Context, a domain.Action) error
	Action(ctx context.Context, id string) (domain.Action, error)
	ActionByExternalID(ctx context.Context, externalID string) (domain.Action, error)
	Actions(ctx context.Context, userID string) ([]domain.Action, error)

	SaveQuestion(ctx context.Context, q domain.Question) error
	Question(ctx context.Context, id string) (domain.Question, error)
	QuestionsForMentor(ctx context.Context, mentorID string) ([]domain.Question, error)
	QuestionsOfIntern(ctx context.Context, internID string) ([]domain.Question, error)

	AddNotification(ctx context.Context, n domain.Notification) error
	Notifications(ctx context.Context, userID string, limit int) ([]domain.Notification, error)
	MarkNotificationsRead(ctx context.Context, userID string) error

	AddFAQ(ctx context.Context, f domain.FAQEntry) error
	FAQs(ctx context.Context) ([]domain.FAQEntry, error)

	SaveGoal(ctx context.Context, g domain.Goal) error
	Goal(ctx context.Context, id string) (domain.Goal, error)
	Goals(ctx context.Context, internID string) ([]domain.Goal, error)
	DeleteGoal(ctx context.Context, id string) error

	AddEvent(ctx context.Context, e domain.Event) error
	Events(ctx context.Context) ([]domain.Event, error)

	// Записи произвольного вида (JSON): инструкции, тесты, шаблоны, программы, права.
	PutRecord(ctx context.Context, kind, id string, data []byte) error
	Record(ctx context.Context, kind, id string) ([]byte, error)
	Records(ctx context.Context, kind string) ([][]byte, error)
	DeleteRecord(ctx context.Context, kind, id string) error
}
