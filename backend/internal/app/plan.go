package app

import (
	"context"

	"aibuddy/internal/domain"
	"aibuddy/internal/events"
	"aibuddy/internal/onboarding"
)

// canViewIntern — наставник, руководитель и HR BP своей команды видят план стажёра.
func canViewIntern(viewer, intern domain.User) bool {
	if viewer.ID == intern.ID {
		return true
	}
	switch viewer.Role {
	case domain.RoleMentor:
		return intern.MentorID == viewer.ID
	case domain.RoleLead:
		return intern.LeadID == viewer.ID
	case domain.RoleHRBP:
		return intern.Team == viewer.Team
	}
	return false
}

// PlanFor — план адаптации пользователя.
func (a *App) PlanFor(ctx context.Context, u domain.User) (onboarding.Plan, error) {
	states, err := a.store.StepStates(ctx, u.ID)
	if err != nil {
		return onboarding.Plan{}, err
	}
	return a.tpl.Build(u, states, a.now()), nil
}

// InternPlan — план стажёра для наставника/руководителя.
func (a *App) InternPlan(ctx context.Context, viewer domain.User, internID string) (onboarding.Plan, domain.User, error) {
	intern, ok := a.userByID(ctx, internID)
	if !ok || intern.Role != domain.RoleIntern {
		return onboarding.Plan{}, intern, ErrNotFound
	}
	if !canViewIntern(viewer, intern) {
		return onboarding.Plan{}, intern, ErrForbidden
	}
	p, err := a.PlanFor(ctx, intern)
	return p, intern, err
}

// SetStep меняет статус шага. Стажёр меняет свои шаги, наставник и
// руководитель — любые шаги своего стажёра.
func (a *App) SetStep(ctx context.Context, actor domain.User, internID, stepID string, status domain.StepStatus, note string) (onboarding.Plan, error) {
	if !domain.ValidStepStatus(status) {
		return onboarding.Plan{}, invalid("Неизвестный статус")
	}
	step, ok := a.tpl.Step(stepID)
	if !ok {
		return onboarding.Plan{}, ErrNotFound
	}
	intern, ok := a.userByID(ctx, internID)
	if !ok {
		return onboarding.Plan{}, ErrNotFound
	}
	switch {
	case actor.ID == intern.ID:
		if step.Owner != "intern" {
			return onboarding.Plan{}, invalid("Этот шаг отмечает наставник")
		}
	case actor.Role == domain.RoleMentor && intern.MentorID == actor.ID, actor.Role == domain.RoleLead && intern.LeadID == actor.ID:
	default:
		return onboarding.Plan{}, ErrForbidden
	}
	if len([]rune(note)) > 500 {
		return onboarding.Plan{}, invalid("Заметка длиннее 500 символов")
	}
	if err := a.store.SetStepState(ctx, domain.StepState{UserID: intern.ID, StepID: stepID, Status: status, Note: note,
		UpdatedAt: a.now(), UpdatedBy: actor.ID}); err != nil {
		return onboarding.Plan{}, err
	}
	if status == domain.StepDone {
		a.Track(ctx, intern.ID, "step_completed", map[string]any{"step": stepID, "by": actor.Role})
	}
	p, err := a.PlanFor(ctx, intern)
	if err == nil {
		a.hub.Publish(intern.ID, events.Update{Type: "plan", Data: p})
	}
	return p, err
}

// autoCompleteSteps отмечает шаги плана, которые закрываются выданными доступами.
func (a *App) autoCompleteSteps(ctx context.Context, intern domain.User) {
	acts, err := a.store.Actions(ctx, intern.ID)
	if err != nil {
		return
	}
	var granted []string
	for _, act := range acts {
		if act.Kind == domain.ActionAccess && act.Status == domain.ActionApproved {
			if sys, ok := act.Args["system"].(string); ok {
				granted = append(granted, sys)
			}
		}
	}
	states, _ := a.store.StepStates(ctx, intern.ID)
	done := map[string]bool{}
	for _, s := range states {
		done[s.StepID] = s.Status == domain.StepDone
	}
	changed := false
	for _, id := range a.tpl.StepsCompletedByAccess(granted) {
		if done[id] {
			continue
		}
		_ = a.store.SetStepState(ctx, domain.StepState{UserID: intern.ID, StepID: id, Status: domain.StepDone,
			Note: "Отмечено автоматически: доступ выдан", UpdatedAt: a.now(), UpdatedBy: "ai-buddy"})
		step, _ := a.tpl.Step(id)
		a.notify(ctx, intern.ID, "plan", "Шаг плана выполнен: "+step.Title, "/plan")
		a.Track(ctx, intern.ID, "step_completed", map[string]any{"step": id, "by": "auto"})
		changed = true
	}
	if changed {
		if p, err := a.PlanFor(ctx, intern); err == nil {
			a.hub.Publish(intern.ID, events.Update{Type: "plan", Data: p})
		}
	}
}
