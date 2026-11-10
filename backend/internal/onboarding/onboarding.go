// Package onboarding — план адаптации стажёра по чек-листу Авито:
// этапы, статусы шагов, просрочки и следующий шаг.
package onboarding

import (
	_ "embed"
	"encoding/json"
	"slices"
	"time"

	"aibuddy/internal/domain"
)

//go:embed template.json
var templateJSON []byte

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
}

// Template — шаблон плана.
type Template struct {
	Periods []Period `json:"periods"`
	Steps   []Step   `json:"steps"`
}

// LoadTemplate загружает встроенный шаблон.
func LoadTemplate() (*Template, error) {
	var t Template
	err := json.Unmarshal(templateJSON, &t)
	return &t, err
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
	Day           int          `json:"day"` // день стажировки, с 1
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
	p := Plan{UserID: u.ID, Day: day}
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
