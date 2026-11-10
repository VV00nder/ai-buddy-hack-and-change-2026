// Package corp — имитация корпоративных сервисов Авито для прототипа:
// справочник сотрудников, система отсутствий и система доступов.
// Все сервисы публикуются как MCP-инструменты и сами проверяют права
// вызывающего пользователя.
package corp

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"aibuddy/internal/domain"
)

//go:embed fixtures/directory.json
var fixturesJSON []byte

// Team — команда.
type Team struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Cluster string `json:"cluster"`
	Chat    string `json:"chat"`
	Curator string `json:"curator"`
}

// Person — запись справочника.
type Person struct {
	domain.User
	Handle          string `json:"handle"`
	StartOffsetDays int    `json:"startOffsetDays"`
}

// System — система в каталоге доступов.
type System struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Description           string   `json:"description"`
	Approval              string   `json:"approval"` // auto | lead
	Levels                []string `json:"levels"`
	JustificationRequired bool     `json:"justificationRequired,omitempty"`
	DeniedRoles           []string `json:"deniedRoles,omitempty"`
	DeniedHint            string   `json:"deniedHint,omitempty"`
}

// Absence — заявка на отсутствие.
type Absence struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	UserName   string     `json:"userName"`
	Type       string     `json:"type"`
	DateFrom   string     `json:"dateFrom"`
	DateTo     string     `json:"dateTo"`
	Days       int        `json:"days"`
	Reason     string     `json:"reason,omitempty"`
	Status     string     `json:"status"` // pending | approved | rejected
	ApproverID string     `json:"approverId"`
	Approver   string     `json:"approver"`
	Comment    string     `json:"comment,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	DecidedAt  *time.Time `json:"decidedAt,omitempty"`
}

// AccessRequest — заявка на доступ.
type AccessRequest struct {
	ID            string     `json:"id"`
	UserID        string     `json:"userId"`
	UserName      string     `json:"userName"`
	System        string     `json:"system"`
	SystemName    string     `json:"systemName"`
	Level         string     `json:"level"`
	Justification string     `json:"justification,omitempty"`
	Status        string     `json:"status"` // granted | pending | rejected
	ApproverID    string     `json:"approverId,omitempty"`
	Approver      string     `json:"approver,omitempty"`
	Comment       string     `json:"comment,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	DecidedAt     *time.Time `json:"decidedAt,omitempty"`
}

type fixtures struct {
	Teams          []Team         `json:"teams"`
	Users          []Person       `json:"users"`
	AbsenceBalance map[string]int `json:"absenceBalance"`
	AccessCatalog  []System       `json:"accessCatalog"`
}

// Service хранит состояние имитаций в памяти.
type Service struct {
	mu       sync.Mutex
	now      func() time.Time
	loc      *time.Location
	teams    map[string]Team
	users    map[string]*Person
	byLogin  map[string]*Person
	balance  map[string]int
	catalog  []System
	absences []*Absence
	access   []*AccessRequest
	seq      int
	// Outage имитирует недоступность сервиса (для демонстрации обработки сбоев).
	outage bool
}

// New создаёт сервис с синтетическими данными. Дата начала стажировки
// считается относительно текущей даты, чтобы демо всегда было «живым».
func New(now func() time.Time, loc *time.Location) (*Service, error) {
	if now == nil {
		now = time.Now
	}
	if loc == nil {
		loc = time.UTC
	}
	var f fixtures
	if err := json.Unmarshal(fixturesJSON, &f); err != nil {
		return nil, fmt.Errorf("corp fixtures: %w", err)
	}
	s := &Service{now: now, loc: loc, teams: map[string]Team{}, users: map[string]*Person{},
		byLogin: map[string]*Person{}, balance: map[string]int{}, catalog: f.AccessCatalog}
	today := dayOf(now().In(loc))
	for _, t := range f.Teams {
		s.teams[t.ID] = t
	}
	for i := range f.Users {
		p := f.Users[i]
		p.StartDate = today.AddDate(0, 0, p.StartOffsetDays)
		if t, ok := s.teams[p.Team]; ok {
			p.TeamName = t.Name
		}
		s.users[p.ID] = &p
		s.byLogin[p.Login] = &p
		if b, ok := f.AbsenceBalance[string(p.Role)]; ok {
			s.balance[p.ID] = b
		} else {
			s.balance[p.ID] = f.AbsenceBalance["default"]
		}
	}
	return s, nil
}

// SetOutage включает или выключает имитацию сбоя.
func (s *Service) SetOutage(v bool) {
	s.mu.Lock()
	s.outage = v
	s.mu.Unlock()
}

// Outage сообщает, включена ли имитация сбоя.
func (s *Service) Outage() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outage
}

// UserByLogin ищет сотрудника по логину.
func (s *Service) UserByLogin(login string) (domain.User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.byLogin[login]
	if !ok {
		return domain.User{}, false
	}
	return p.User, true
}

// Users возвращает всех сотрудников (для страницы входа с тестовыми учётками).
func (s *Service) Users() []domain.User {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.User, 0, len(s.users))
	for _, l := range []string{"anna", "ivan", "oleg", "maria", "dmitry", "sergey", "elena"} {
		if p, ok := s.byLogin[l]; ok {
			out = append(out, p.User)
		}
	}
	return out
}

func (s *Service) nextID(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s-%04d", prefix, 1000+s.seq)
}

func (s *Service) today() time.Time { return dayOf(s.now().In(s.loc)) }
