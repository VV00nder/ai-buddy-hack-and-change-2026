// Package onboarding — план адаптации стажёра по чек-листу Авито:
// этапы, статусы шагов, просрочки и следующий шаг.
package onboarding

import (
	"embed"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"aibuddy/internal/domain"
)

//go:embed templates/*.json
var templatesFS embed.FS

// DefaultID — шаблон, по которому строится план, если стажёру не назначен другой.
const DefaultID = "intern-dev"

// Period — этап адаптации.
type Period struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	FromDay int    `json:"fromDay"`
	ToDay   int    `json:"toDay"`
}

// Auto — условие автоматического выполнения шага.
type Auto struct {
	Access []string `json:"access,omitempty"` // шаг выполнен, когда выданы все эти доступы
}

// Step — шаг шаблона.
type Step struct {
	ID          string   `json:"id"`
	Period      string   `json:"period"`
	Owner       string   `json:"owner"` // intern | mentor
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Docs        []string `json:"docs,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
	Auto        *Auto    `json:"auto,omitempty"`
	// Quiz — тест, сдача которого закрывает шаг.
	Quiz string `json:"quiz,omitempty"`
}

// Template — шаблон программы обучения.
type Template struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Team        string   `json:"team,omitempty"` // команда автора; пусто — встроенный шаблон для всех
	Builtin     bool     `json:"builtin"`
	UpdatedBy   string   `json:"updatedBy,omitempty"`
	UpdatedAt   string   `json:"updatedAt,omitempty"`
	Periods     []Period `json:"periods"`
	Steps       []Step   `json:"steps"`
}

// LoadTemplates загружает встроенные шаблоны (первым идёт шаблон по умолчанию).
func LoadTemplates() ([]*Template, error) {
	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		return nil, err
	}
	var out []*Template
	for _, e := range entries {
		raw, err := templatesFS.ReadFile("templates/" + e.Name())
		if err != nil {
			return nil, err
		}
		var t Template
		if err := json.Unmarshal(raw, &t); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		t.Builtin = true
		if err := t.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, &t)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID == DefaultID && out[j].ID != DefaultID })
	return out, nil
}

// LoadTemplate загружает шаблон по умолчанию.
func LoadTemplate() (*Template, error) {
	all, err := LoadTemplates()
	if err != nil {
		return nil, err
	}
	return all[0], nil
}

// Clone — глубокая копия шаблона.
func (t *Template) Clone() *Template {
	c := *t
	c.Periods = slices.Clone(t.Periods)
	c.Steps = make([]Step, len(t.Steps))
	for i, s := range t.Steps {
		s.Docs = slices.Clone(s.Docs)
		if s.Auto != nil {
			a := Auto{Access: slices.Clone(s.Auto.Access)}
			s.Auto = &a
		}
		c.Steps[i] = s
	}
	return &c
}

// Validate проверяет шаблон: название, этапы, уникальность и заполненность шагов.
func (t *Template) Validate() error {
	if n := utf8.RuneCountInString(strings.TrimSpace(t.Name)); n < 3 || n > 120 {
		return fmt.Errorf("Название шаблона — от 3 до 120 символов")
	}
	if len(t.Periods) == 0 {
		return fmt.Errorf("В шаблоне нет этапов")
	}
	periods := map[string]bool{}
	for _, p := range t.Periods {
		if p.ID == "" || strings.TrimSpace(p.Name) == "" || p.ToDay < p.FromDay {
			return fmt.Errorf("Этап «%s» заполнен неверно", p.Name)
		}
		periods[p.ID] = true
	}
	if len(t.Steps) == 0 {
		return fmt.Errorf("Добавьте хотя бы один шаг")
	}
	if len(t.Steps) > 200 {
		return fmt.Errorf("Слишком много шагов (больше 200)")
	}
	ids := map[string]bool{}
	for _, s := range t.Steps {
		switch {
		case s.ID == "" || ids[s.ID]:
			return fmt.Errorf("Повторяющийся или пустой идентификатор шага «%s»", s.Title)
		case !periods[s.Period]:
			return fmt.Errorf("У шага «%s» неизвестный этап", s.Title)
		case s.Owner != "intern" && s.Owner != "mentor":
			return fmt.Errorf("У шага «%s» неизвестный исполнитель", s.Title)
		case utf8.RuneCountInString(strings.TrimSpace(s.Title)) < 3 || utf8.RuneCountInString(s.Title) > 200:
			return fmt.Errorf("Название шага — от 3 до 200 символов")
		case utf8.RuneCountInString(s.Description) > 2000:
			return fmt.Errorf("Описание шага «%s» длиннее 2000 символов", s.Title)
		}
		ids[s.ID] = true
	}
	return nil
}

// StepView — шаг плана с состоянием для конкретного стажёра.
type StepView struct {
	Step
	Status    domain.StepStatus `json:"status"`
	Note      string            `json:"note,omitempty"`
	UpdatedAt *time.Time        `json:"updatedAt,omitempty"`
	Due       string            `json:"due"`
	Overdue   bool              `json:"overdue"`
}

// PeriodView — этап с прогрессом.
type PeriodView struct {
	Period
	State string     `json:"state"` // past | current | future
	Done  int        `json:"done"`
	Total int        `json:"total"`
	Steps []StepView `json:"steps"`
}

// Plan — план адаптации.
type Plan struct {
	UserID        string       `json:"userId"`
	TemplateID    string       `json:"templateId"`
	TemplateName  string       `json:"templateName"`
	Customized    bool         `json:"customized"` // программа изменена под стажёра
	Day           int          `json:"day"`        // день стажировки, с 1
	CurrentPeriod string       `json:"currentPeriod"`
	Done          int          `json:"done"`
	Total         int          `json:"total"`
	Percent       int          `json:"percent"`
	Overdue       int          `json:"overdue"`
	Next          *StepView    `json:"next,omitempty"`
	Periods       []PeriodView `json:"periods"`
}

// Build собирает план стажёра на дату now.
func (t *Template) Build(u domain.User, states []domain.StepState, now time.Time) Plan {
	byStep := map[string]domain.StepState{}
	for _, s := range states {
		byStep[s.StepID] = s
	}
	start := dayOf(u.StartDate.In(now.Location()))
	day := int(dayOf(now).Sub(start).Hours()/24) + 1
	if day < 1 {
		day = 1
	}
	p := Plan{UserID: u.ID, Day: day, TemplateID: t.ID, TemplateName: t.Name}
	for _, per := range t.Periods {
		pv := PeriodView{Period: per, State: "future"}
		switch {
		case day-1 > per.ToDay:
			pv.State = "past"
		case day-1 >= per.FromDay:
			pv.State = "current"
			p.CurrentPeriod = per.ID
		}
		due := start.AddDate(0, 0, per.ToDay)
		for _, st := range t.Steps {
			if st.Period != per.ID {
				continue
			}
			sv := StepView{Step: st, Status: domain.StepTodo, Due: due.Format("2006-01-02")}
			if s, ok := byStep[st.ID]; ok {
				sv.Status, sv.Note = s.Status, s.Note
				at := s.UpdatedAt
				sv.UpdatedAt = &at
			}
			if sv.Status == domain.StepDone {
				pv.Done++
			} else if pv.State == "past" {
				sv.Overdue = true
				p.Overdue++
			}
			pv.Total++
			pv.Steps = append(pv.Steps, sv)
		}
		p.Done += pv.Done
		p.Total += pv.Total
		p.Periods = append(p.Periods, pv)
	}
	if p.Total > 0 {
		p.Percent = p.Done * 100 / p.Total
	}
	p.Next = nextStep(p)
	return p
}

// nextStep: сначала просроченные и начатые, затем первый невыполненный шаг
// текущего этапа, который делает сам стажёр.
func nextStep(p Plan) *StepView {
	var cands []StepView
	for _, per := range p.Periods {
		if per.State == "future" {
			continue
		}
		for _, s := range per.Steps {
			if s.Owner == "intern" && s.Status != domain.StepDone {
				cands = append(cands, s)
			}
		}
	}
	if len(cands) == 0 {
		for _, per := range p.Periods {
			for _, s := range per.Steps {
				if s.Owner == "intern" && s.Status != domain.StepDone {
					return &s
				}
			}
		}
		return nil
	}
	rank := func(s StepView) int {
		switch {
		case s.Status == domain.StepInProgress:
			return 0
		case s.Overdue:
			return 1
		case s.Status == domain.StepBlocked:
			return 3
		}
		return 2
	}
	slices.SortStableFunc(cands, func(a, b StepView) int { return rank(a) - rank(b) })
	return &cands[0]
}

// StepsCompletedByAccess возвращает шаги, которые становятся выполненными,
// когда у пользователя выданы доступы granted.
func (t *Template) StepsCompletedByAccess(granted []string) []string {
	var out []string
	for _, s := range t.Steps {
		if s.Auto == nil || len(s.Auto.Access) == 0 {
			continue
		}
		all := true
		for _, need := range s.Auto.Access {
			if !slices.Contains(granted, need) {
				all = false
				break
			}
		}
		if all {
			out = append(out, s.ID)
		}
	}
	return out
}

// StepsWithQuiz — шаги, которые закрываются сдачей теста.
func (t *Template) StepsWithQuiz(quizID string) []string {
	var out []string
	for _, s := range t.Steps {
		if s.Quiz == quizID {
			out = append(out, s.ID)
		}
	}
	return out
}

// Step возвращает шаг по id.
func (t *Template) Step(id string) (Step, bool) {
	for _, s := range t.Steps {
		if s.ID == id {
			return s, true
		}
	}
	return Step{}, false
}

func dayOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
