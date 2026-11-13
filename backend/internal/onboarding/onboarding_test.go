package onboarding

import (
	"strings"
	"testing"
	"time"

	"aibuddy/internal/domain"
)

var msk = time.FixedZone("MSK", 3*3600)

func TestBuildPlan(t *testing.T) {
	tpl, err := LoadTemplate()
	if err != nil || len(tpl.Steps) < 20 || len(tpl.Periods) != 6 {
		t.Fatalf("template: %v", err)
	}
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, msk)
	u := domain.User{ID: "u", StartDate: time.Date(2026, 11, 7, 0, 0, 0, 0, msk)}
	p := tpl.Build(u, nil, now)
	if p.Day != 4 || p.CurrentPeriod != "week1" || p.Done != 0 || p.Overdue != 0 {
		t.Fatalf("plan: day=%d period=%s", p.Day, p.CurrentPeriod)
	}
	if p.Next == nil || p.Next.ID != "w1-team" {
		t.Fatalf("next: %+v", p.Next)
	}
	states := []domain.StepState{
		{StepID: "w1-team", Status: domain.StepDone, UpdatedAt: now},
		{StepID: "w1-avito-cli", Status: domain.StepInProgress, UpdatedAt: now, Note: "жду VPN"},
	}
	p = tpl.Build(u, states, now)
	if p.Done != 1 || p.Next.ID != "w1-avito-cli" || p.Next.Note != "жду VPN" || p.Percent == 0 {
		t.Fatalf("in progress first: %+v", p.Next)
	}
	// через 20 дней: этапы week1, week2 прошли — шаги просрочены
	later := tpl.Build(u, states, now.AddDate(0, 0, 20))
	if later.CurrentPeriod != "month1" || later.Overdue == 0 {
		t.Fatalf("overdue: %+v", later.Overdue)
	}
	if later.Periods[0].State != "past" || later.Periods[2].State != "current" || later.Periods[5].State != "future" {
		t.Fatal("period states")
	}
	// все шаги выполнены — next nil
	var all []domain.StepState
	for _, s := range tpl.Steps {
		all = append(all, domain.StepState{StepID: s.ID, Status: domain.StepDone, UpdatedAt: now})
	}
	if done := tpl.Build(u, all, now); done.Next != nil || done.Percent != 100 {
		t.Fatal("all done")
	}
	// всё текущее выполнено — next берётся из будущих этапов
	var week1 []domain.StepState
	for _, s := range tpl.Steps {
		if s.Period == "week1" {
			week1 = append(week1, domain.StepState{StepID: s.ID, Status: domain.StepDone, UpdatedAt: now})
		}
	}
	if n := tpl.Build(u, week1, now).Next; n == nil || n.Period != "week2" {
		t.Fatalf("future next: %+v", n)
	}
	// заблокированный шаг уходит в конец
	blocked := []domain.StepState{{StepID: "w1-team", Status: domain.StepBlocked, UpdatedAt: now}}
	if n := tpl.Build(u, blocked, now).Next; n.ID == "w1-team" {
		t.Fatal("blocked must not be first")
	}
	// старт в будущем
	if f := tpl.Build(domain.User{StartDate: now.AddDate(0, 0, 5)}, nil, now); f.Day != 1 {
		t.Fatal("future start")
	}
	if (&Template{}).Build(u, nil, now).Percent != 0 {
		t.Fatal("empty template")
	}
}

func TestAutoAndLookup(t *testing.T) {
	tpl, _ := LoadTemplate()
	got := tpl.StepsCompletedByAccess([]string{"vpn", "jira", "stash"})
	if len(got) != 1 || got[0] != "w1-stash" {
		t.Fatalf("partial: %v", got)
	}
	got = tpl.StepsCompletedByAccess([]string{"vpn", "jira", "confluence-team", "stash"})
	if len(got) != 2 {
		t.Fatalf("full: %v", got)
	}
	if s, ok := tpl.Step("w1-stash"); !ok || s.Owner != "intern" {
		t.Fatal("step")
	}
	if _, ok := tpl.Step("none"); ok {
		t.Fatal("none")
	}
}

func TestTemplates(t *testing.T) {
	all, err := LoadTemplates()
	if err != nil || len(all) != 2 || all[0].ID != DefaultID || !all[0].Builtin {
		t.Fatalf("templates: %v %d", err, len(all))
	}
	c := all[0].Clone()
	c.Steps[0].Title = "changed"
	c.Steps[0].Docs = append(c.Steps[0].Docs, "x")
	if all[0].Steps[0].Title == "changed" {
		t.Fatal("clone shares steps")
	}
	if ids := all[0].StepsWithQuiz("quiz-week1"); len(ids) != 1 || ids[0] != "w1-quiz" {
		t.Fatalf("quiz steps: %v", ids)
	}
	bad := func(mut func(t *Template)) error {
		x := all[0].Clone()
		mut(x)
		return x.Validate()
	}
	for name, mut := range map[string]func(t *Template){
		"name":      func(t *Template) { t.Name = "x" },
		"periods":   func(t *Template) { t.Periods = nil },
		"period":    func(t *Template) { t.Periods[0].ToDay = -1 },
		"steps":     func(t *Template) { t.Steps = nil },
		"dup":       func(t *Template) { t.Steps[1].ID = t.Steps[0].ID },
		"unknown":   func(t *Template) { t.Steps[0].Period = "nope" },
		"owner":     func(t *Template) { t.Steps[0].Owner = "boss" },
		"title":     func(t *Template) { t.Steps[0].Title = "x" },
		"long desc": func(t *Template) { t.Steps[0].Description = strings.Repeat("я", 2001) },
		"many":      func(t *Template) { t.Steps = make([]Step, 201) },
	} {
		if bad(mut) == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
