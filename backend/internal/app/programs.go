package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aibuddy/internal/domain"
	"aibuddy/internal/events"
	"aibuddy/internal/onboarding"
)

// Шаблоны обучения и программы стажёров. Шаблон — набор этапов и шагов;
// программа стажёра — копия шаблона, которую наставник или руководитель
// может настроить под конкретного человека, не меняя шаблон.

// Program — программа обучения стажёра.
type Program struct {
	InternID   string               `json:"internId"`
	BaseID     string               `json:"baseId"` // из какого шаблона создана
	Customized bool                 `json:"customized"`
	Template   *onboarding.Template `json:"template"`
	UpdatedBy  string               `json:"updatedBy,omitempty"`
	UpdatedAt  string               `json:"updatedAt,omitempty"`
}

// templateFor — программа стажёра или шаблон по умолчанию.
func (a *App) templateFor(ctx context.Context, intern domain.User) (*onboarding.Template, bool) {
	if p, err := getRec[Program](ctx, a.store, recProgram, intern.ID); err == nil && p.Template != nil {
		return p.Template, p.Customized
	}
	return a.tpl, false
}

// Templates — встроенные шаблоны и шаблоны своей команды.
func (a *App) Templates(ctx context.Context, u domain.User) ([]*onboarding.Template, error) {
	if u.Role == domain.RoleIntern {
		return nil, ErrForbidden
	}
	out := []*onboarding.Template{}
	out = append(out, a.builtinTpl...)
	custom, err := listRec[*onboarding.Template](ctx, a.store, recTemplate)
	if err != nil {
		return nil, err
	}
	for _, t := range custom {
		if t.Team == u.Team || t.Team == "all" || isAdmin(u) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (a *App) templateByID(ctx context.Context, u domain.User, id string) (*onboarding.Template, error) {
	all, err := a.Templates(ctx, u)
	if err != nil {
		return nil, err
	}
	for _, t := range all {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, ErrNotFound
}

// normalizeSteps проставляет идентификаторы новым шагам и обрезает пробелы.
func normalizeSteps(steps []onboarding.Step) []onboarding.Step {
	out := make([]onboarding.Step, 0, len(steps))
	for _, s := range steps {
		if strings.TrimSpace(s.ID) == "" {
			s.ID = newID("st")
		}
		s.Title, s.Description, s.Category = strings.TrimSpace(s.Title), strings.TrimSpace(s.Description), strings.TrimSpace(s.Category)
		if s.Owner == "" {
			s.Owner = "intern"
		}
		if s.Category == "" {
			s.Category = "Обучение"
		}
		out = append(out, s)
	}
	return out
}

// SaveTemplate создаёт или изменяет шаблон команды. Встроенные шаблоны не меняются —
// их можно скопировать.
func (a *App) SaveTemplate(ctx context.Context, u domain.User, in onboarding.Template) (*onboarding.Template, error) {
	if !canManagePrograms(u) {
		return nil, ErrForbidden
	}
	team := contentTeam(u)
	if in.ID == "" {
		in.ID = newID("tpl")
	} else {
		cur, err := a.templateByID(ctx, u, in.ID)
		if err != nil {
			return nil, err
		}
		if cur.Builtin {
			return nil, invalid("Встроенный шаблон нельзя изменить — сделайте копию")
		}
		if !sameTeam(u, cur.Team) {
			return nil, ErrForbidden
		}
		team = cur.Team
	}
	if len(in.Periods) == 0 {
		in.Periods = a.tpl.Clone().Periods
	}
	t := &onboarding.Template{ID: in.ID, Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(in.Description), Team: team,
		UpdatedBy: u.Name, UpdatedAt: a.now().Format(time.RFC3339), Periods: in.Periods, Steps: normalizeSteps(in.Steps)}
	if err := a.checkTemplate(ctx, u, t); err != nil {
		return nil, err
	}
	if err := putRec(ctx, a.store, recTemplate, t.ID, t); err != nil {
		return nil, err
	}
	a.Track(ctx, u.ID, "template_saved", map[string]any{"template": t.ID, "steps": len(t.Steps)})
	return t, nil
}

func (a *App) checkTemplate(ctx context.Context, u domain.User, t *onboarding.Template) error {
	if err := t.Validate(); err != nil {
		return invalid("%s", err.Error())
	}
	for _, s := range t.Steps {
		if s.Quiz != "" {
			if _, err := a.quizByID(ctx, u, s.Quiz); err != nil {
				return invalid("У шага «%s» указан неизвестный тест", s.Title)
			}
		}
	}
	return nil
}

// DeleteTemplate удаляет шаблон команды. Программы стажёров — копии и остаются.
func (a *App) DeleteTemplate(ctx context.Context, u domain.User, id string) error {
	if !canManagePrograms(u) {
		return ErrForbidden
	}
	t, err := a.templateByID(ctx, u, id)
	if err != nil {
		return err
	}
	if t.Builtin {
		return invalid("Встроенный шаблон нельзя удалить")
	}
	if !sameTeam(u, t.Team) {
		return ErrForbidden
	}
	return a.store.DeleteRecord(ctx, recTemplate, id)
}

// canEditProgram — программу стажёра настраивают его руководитель и наставник.
func canEditProgram(actor, intern domain.User) bool {
	return canSetGoals(actor, intern)
}

// ProgramOf — текущая программа стажёра (для редактора).
func (a *App) ProgramOf(ctx context.Context, viewer domain.User, internID string) (Program, error) {
	intern, ok := a.userByID(ctx, internID)
	if !ok || intern.Role != domain.RoleIntern {
		return Program{}, ErrNotFound
	}
	if !canViewIntern(viewer, intern) {
		return Program{}, ErrForbidden
	}
	if p, err := getRec[Program](ctx, a.store, recProgram, intern.ID); err == nil && p.Template != nil {
		return p, nil
	}
	return Program{InternID: intern.ID, BaseID: a.tpl.ID, Template: a.tpl}, nil
}

// AssignTemplate назначает стажёру шаблон: программа становится копией шаблона.
// Отметки о выполненных шагах сохраняются для совпадающих шагов.
func (a *App) AssignTemplate(ctx context.Context, actor domain.User, internID, templateID string) (Program, error) {
	intern, ok := a.userByID(ctx, internID)
	if !ok || intern.Role != domain.RoleIntern {
		return Program{}, ErrNotFound
	}
	if !canEditProgram(actor, intern) {
		return Program{}, ErrForbidden
	}
	t, err := a.templateByID(ctx, actor, templateID)
	if err != nil {
		return Program{}, err
	}
	c := t.Clone()
	c.Builtin = false
	p := Program{InternID: intern.ID, BaseID: t.ID, Template: c, UpdatedBy: actor.Name, UpdatedAt: a.now().Format(time.RFC3339)}
	if err := putRec(ctx, a.store, recProgram, intern.ID, p); err != nil {
		return Program{}, err
	}
	a.notify(ctx, intern.ID, "plan", fmt.Sprintf("%s назначил(а) тебе программу обучения «%s»", actor.Name, t.Name), "/plan")
	a.Track(ctx, actor.ID, "program_assigned", map[string]any{"intern": intern.ID, "template": t.ID})
	a.publishPlan(ctx, intern)
	return p, nil
}

// SaveProgram сохраняет программу, настроенную под стажёра.
func (a *App) SaveProgram(ctx context.Context, actor domain.User, internID string, steps []onboarding.Step) (Program, error) {
	intern, ok := a.userByID(ctx, internID)
	if !ok || intern.Role != domain.RoleIntern {
		return Program{}, ErrNotFound
	}
	if !canEditProgram(actor, intern) {
		return Program{}, ErrForbidden
	}
	cur, err := a.ProgramOf(ctx, actor, internID)
	if err != nil {
		return Program{}, err
	}
	t := cur.Template.Clone()
	t.Builtin = false
	t.Steps = normalizeSteps(steps)
	if err := a.checkTemplate(ctx, actor, t); err != nil {
		return Program{}, err
	}
	p := Program{InternID: intern.ID, BaseID: cur.BaseID, Customized: true, Template: t, UpdatedBy: actor.Name, UpdatedAt: a.now().Format(time.RFC3339)}
	if err := putRec(ctx, a.store, recProgram, intern.ID, p); err != nil {
		return Program{}, err
	}
	a.notify(ctx, intern.ID, "plan", actor.Name+" обновил(а) твою программу обучения", "/plan")
	a.Track(ctx, actor.ID, "program_customized", map[string]any{"intern": intern.ID, "steps": len(t.Steps)})
	a.publishPlan(ctx, intern)
	return p, nil
}

func (a *App) publishPlan(ctx context.Context, intern domain.User) {
	if p, err := a.PlanFor(ctx, intern); err == nil {
		a.hub.Publish(intern.ID, events.Update{Type: "plan", Data: p})
	}
}
