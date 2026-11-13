package store

import (
	"context"
	"sort"
	"sync"

	"aibuddy/internal/domain"
)

// Memory — потокобезопасное хранилище в памяти.
type Memory struct {
	mu        sync.RWMutex
	steps     map[string]map[string]domain.StepState
	messages  map[string]domain.Message
	actions   map[string]domain.Action
	questions map[string]domain.Question
	notifs    []domain.Notification
	faqs      []domain.FAQEntry
	goals     map[string]domain.Goal
	events    []domain.Event
	records   map[string]map[string][]byte
}

// NewMemory создаёт пустое хранилище.
func NewMemory() *Memory {
	return &Memory{steps: map[string]map[string]domain.StepState{}, messages: map[string]domain.Message{},
		actions: map[string]domain.Action{}, questions: map[string]domain.Question{}, goals: map[string]domain.Goal{}, records: map[string]map[string][]byte{}}
}

// Ping всегда успешен.
func (m *Memory) Ping(context.Context) error { return nil }

func (m *Memory) StepStates(_ context.Context, userID string) ([]domain.StepState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []domain.StepState{}
	for _, s := range m.steps[userID] {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StepID < out[j].StepID })
	return out, nil
}

func (m *Memory) SetStepState(_ context.Context, s domain.StepState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.steps[s.UserID] == nil {
		m.steps[s.UserID] = map[string]domain.StepState{}
	}
	m.steps[s.UserID][s.StepID] = s
	return nil
}

func (m *Memory) SaveMessage(_ context.Context, msg domain.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[msg.ID] = msg
	return nil
}

func (m *Memory) Message(_ context.Context, id string) (domain.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	msg, ok := m.messages[id]
	if !ok {
		return msg, ErrNotFound
	}
	return msg, nil
}

func (m *Memory) Messages(_ context.Context, userID string, limit int) ([]domain.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []domain.Message{}
	for _, msg := range m.messages {
		if msg.UserID == userID {
			out = append(out, msg)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (m *Memory) SaveAction(_ context.Context, a domain.Action) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actions[a.ID] = a
	return nil
}

func (m *Memory) Action(_ context.Context, id string) (domain.Action, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.actions[id]
	if !ok {
		return a, ErrNotFound
	}
	return a, nil
}

func (m *Memory) ActionByExternalID(_ context.Context, ext string) (domain.Action, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, a := range m.actions {
		if ext != "" && a.ExternalID == ext {
			return a, nil
		}
	}
	return domain.Action{}, ErrNotFound
}

func (m *Memory) Actions(_ context.Context, userID string) ([]domain.Action, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []domain.Action{}
	for _, a := range m.actions {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (m *Memory) SaveQuestion(_ context.Context, q domain.Question) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.questions[q.ID] = q
	return nil
}

func (m *Memory) Question(_ context.Context, id string) (domain.Question, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	q, ok := m.questions[id]
	if !ok {
		return q, ErrNotFound
	}
	return q, nil
}

func (m *Memory) filterQuestions(keep func(domain.Question) bool) []domain.Question {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []domain.Question{}
	for _, q := range m.questions {
		if keep(q) {
			out = append(out, q)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (m *Memory) QuestionsForMentor(_ context.Context, id string) ([]domain.Question, error) {
	return m.filterQuestions(func(q domain.Question) bool { return q.MentorID == id }), nil
}

func (m *Memory) QuestionsOfIntern(_ context.Context, id string) ([]domain.Question, error) {
	return m.filterQuestions(func(q domain.Question) bool { return q.InternID == id }), nil
}

func (m *Memory) AddNotification(_ context.Context, n domain.Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifs = append(m.notifs, n)
	return nil
}

func (m *Memory) Notifications(_ context.Context, userID string, limit int) ([]domain.Notification, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []domain.Notification{}
	for i := len(m.notifs) - 1; i >= 0; i-- {
		if m.notifs[i].UserID == userID {
			out = append(out, m.notifs[i])
			if limit > 0 && len(out) == limit {
				break
			}
		}
	}
	return out, nil
}

func (m *Memory) MarkNotificationsRead(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.notifs {
		if m.notifs[i].UserID == userID {
			m.notifs[i].Read = true
		}
	}
	return nil
}

func (m *Memory) AddFAQ(_ context.Context, f domain.FAQEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.faqs = append(m.faqs, f)
	return nil
}

func (m *Memory) FAQs(context.Context) ([]domain.FAQEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.FAQEntry{}, m.faqs...), nil
}

func (m *Memory) SaveGoal(_ context.Context, g domain.Goal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.goals[g.ID] = g
	return nil
}

func (m *Memory) Goal(_ context.Context, id string) (domain.Goal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	g, ok := m.goals[id]
	if !ok {
		return g, ErrNotFound
	}
	return g, nil
}

func (m *Memory) Goals(_ context.Context, internID string) ([]domain.Goal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []domain.Goal{}
	for _, g := range m.goals {
		if g.InternID == internID {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (m *Memory) AddEvent(_ context.Context, e domain.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, e)
	return nil
}

func (m *Memory) Events(context.Context) ([]domain.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.Event{}, m.events...), nil
}

func (m *Memory) PutRecord(_ context.Context, kind, id string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.records[kind] == nil {
		m.records[kind] = map[string][]byte{}
	}
	m.records[kind][id] = append([]byte(nil), data...)
	return nil
}

func (m *Memory) Record(_ context.Context, kind, id string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.records[kind][id]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), b...), nil
}

func (m *Memory) Records(_ context.Context, kind string) ([][]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.records[kind]))
	for id := range m.records[kind] {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := [][]byte{}
	for _, id := range ids {
		out = append(out, append([]byte(nil), m.records[kind][id]...))
	}
	return out, nil
}

func (m *Memory) DeleteRecord(_ context.Context, kind, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.records[kind], id)
	return nil
}
