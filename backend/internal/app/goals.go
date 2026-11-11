package app

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"aibuddy/internal/domain"
	"aibuddy/internal/events"
)

// Цели стажировки: на первой неделе руководитель согласует со стажёром цели
// и критерии успеха, стажёр ведёт по ним прогресс (трекер из чек-листа).

// GoalInput — поля цели.
type GoalInput struct {
	Title  string `json:"title"`
	Metric string `json:"metric"`
	Due    string `json:"due"`
}

// GoalUpdate — изменение статуса и прогресса.
type GoalUpdate struct {
	Status   domain.GoalStatus `json:"status"`
	Progress *int              `json:"progress"`
	Note     *string           `json:"note"`
}

func canSetGoals(actor, intern domain.User) bool {
	return (actor.Role == domain.RoleLead && intern.LeadID == actor.ID) || (actor.Role == domain.RoleMentor && intern.MentorID == actor.ID)
}

// Goals — цели стажёра.
func (a *App) Goals(ctx context.Context, viewer domain.User, internID string) ([]domain.Goal, error) {
	if internID == "" {
		internID = viewer.ID
	}
	intern, ok := a.userByID(ctx, internID)
	if !ok || intern.Role != domain.RoleIntern {
		return nil, ErrNotFound
	}
	if !canViewIntern(viewer, intern) {
		return nil, ErrForbidden
	}
	return a.store.Goals(ctx, internID)
}

// AddGoal — руководитель или наставник добавляет цель стажёру.
func (a *App) AddGoal(ctx context.Context, actor domain.User, internID string, in GoalInput) (domain.Goal, error) {
	intern, ok := a.userByID(ctx, internID)
	if !ok || intern.Role != domain.RoleIntern {
		return domain.Goal{}, ErrNotFound
	}
	if !canSetGoals(actor, intern) {
		return domain.Goal{}, ErrForbidden
	}
	in.Title, in.Metric = strings.TrimSpace(in.Title), strings.TrimSpace(in.Metric)
	if n := utf8.RuneCountInString(in.Title); n < 5 || n > 200 {
		return domain.Goal{}, invalid("Цель — от 5 до 200 символов")
	}
	if n := utf8.RuneCountInString(in.Metric); n < 5 || n > 300 {
		return domain.Goal{}, invalid("Опишите измеримый критерий успеха (от 5 до 300 символов)")
	}
	if _, err := time.Parse("2006-01-02", in.Due); err != nil {
		return domain.Goal{}, invalid("Срок в формате ГГГГ-ММ-ДД")
	}
	now := a.now()
	g := domain.Goal{ID: newID("g"), InternID: intern.ID, Title: in.Title, Metric: in.Metric, Due: in.Due,
		Status: domain.GoalAgreed, CreatedBy: actor.ID, CreatedAt: a.stamp(), UpdatedAt: now}
	if err := a.store.SaveGoal(ctx, g); err != nil {
		return g, err
	}
	a.notify(ctx, intern.ID, "plan", fmt.Sprintf("%s добавил(а) цель стажировки: %s", actor.Name, g.Title), "/plan")
	a.hub.Publish(intern.ID, events.Update{Type: "goal", Data: g})
	a.Track(ctx, intern.ID, "goal_created", map[string]any{"by": actor.Role})
	// шаг плана «Обсудить цели и план адаптации» закрывается, когда цели поставлены
	if states, err := a.store.StepStates(ctx, intern.ID); err == nil {
		done := false
		for _, s := range states {
			done = done || (s.StepID == "w1-goals" && s.Status == domain.StepDone)
		}
		if !done {
			_ = a.store.SetStepState(ctx, domain.StepState{UserID: intern.ID, StepID: "w1-goals", Status: domain.StepDone,
				Note: "Отмечено автоматически: цели поставлены", UpdatedAt: now, UpdatedBy: "ai-buddy"})
			a.Track(ctx, intern.ID, "step_completed", map[string]any{"step": "w1-goals", "by": "auto"})
		}
	}
	return g, nil
}

// UpdateGoal — стажёр отмечает прогресс; руководитель и наставник тоже могут.
func (a *App) UpdateGoal(ctx context.Context, actor domain.User, id string, in GoalUpdate) (domain.Goal, error) {
	g, err := a.store.Goal(ctx, id)
	if err != nil {
		return g, ErrNotFound
	}
	intern, ok := a.userByID(ctx, g.InternID)
	if !ok {
		return g, ErrNotFound
	}
	if actor.ID != intern.ID && !canSetGoals(actor, intern) {
		return g, ErrForbidden
	}
	switch in.Status {
	case "":
	case domain.GoalAgreed, domain.GoalInProgress, domain.GoalDone:
		g.Status = in.Status
	default:
		return g, invalid("Неизвестный статус цели")
	}
	if in.Progress != nil {
		if *in.Progress < 0 || *in.Progress > 100 {
			return g, invalid("Прогресс — от 0 до 100")
		}
		g.Progress = *in.Progress
		if g.Progress > 0 && g.Status == domain.GoalAgreed {
			g.Status = domain.GoalInProgress
		}
	}
	if g.Status == domain.GoalDone {
		g.Progress = 100
	}
	if in.Note != nil {
		if utf8.RuneCountInString(*in.Note) > 500 {
			return g, invalid("Комментарий длиннее 500 символов")
		}
		g.Note = strings.TrimSpace(*in.Note)
	}
	g.UpdatedAt = a.now()
	if err := a.store.SaveGoal(ctx, g); err != nil {
		return g, err
	}
	a.hub.Publish(intern.ID, events.Update{Type: "goal", Data: g})
	if g.Status == domain.GoalDone {
		a.Track(ctx, intern.ID, "goal_done", nil)
		if actor.ID == intern.ID && intern.LeadID != "" {
			a.notify(ctx, intern.LeadID, "plan", fmt.Sprintf("%s отметил(а) цель выполненной: %s", intern.Name, g.Title), "/team")
		}
	}
	return g, nil
}

// SeedGoals — стартовые цели для демо-стажёров (синтетические данные), если целей ещё нет.
func (a *App) SeedGoals(ctx context.Context, seeds map[string][]GoalInput, by map[string]string) error {
	for intern, list := range seeds {
		existing, err := a.store.Goals(ctx, intern)
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			continue
		}
		for _, in := range list {
			g := domain.Goal{ID: newID("g"), InternID: intern, Title: in.Title, Metric: in.Metric, Due: in.Due,
				Status: domain.GoalAgreed, CreatedBy: by[intern], CreatedAt: a.stamp(), UpdatedAt: a.now()}
			if err := a.store.SaveGoal(ctx, g); err != nil {
				return err
			}
		}
		if err := a.store.SetStepState(ctx, domain.StepState{UserID: intern, StepID: "w1-goals", Status: domain.StepDone,
			Note: "Цели согласованы с руководителем", UpdatedAt: a.now(), UpdatedBy: by[intern]}); err != nil {
			return err
		}
	}
	return nil
}
