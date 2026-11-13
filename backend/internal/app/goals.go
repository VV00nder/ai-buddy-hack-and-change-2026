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

// GoalInput — поля цели: задачи, по закрытию которых считается прогресс.
type GoalInput struct {
	Title  string      `json:"title"`
	Metric string      `json:"metric"`
	Due    string      `json:"due"`
	Tasks  []TaskInput `json:"tasks"`
}

// TaskInput — задача цели: свой пункт или шаг плана стажёра.
type TaskInput struct {
	Title  string `json:"title"`
	StepID string `json:"stepId"`
}

// GoalUpdate — комментарий к цели (прогресс и статус считаются по задачам).
type GoalUpdate struct {
	Note *string `json:"note"`
}

// withProgress пересчитывает задачи-шаги по плану, процент и статус цели.
func withProgress(g domain.Goal, stepDone map[string]bool) domain.Goal {
	done := 0
	tasks := make([]domain.GoalTask, len(g.Tasks))
	for i, t := range g.Tasks {
		if t.StepID != "" {
			t.Done = stepDone[t.StepID]
		}
		if t.Done {
			done++
		}
		tasks[i] = t
	}
	g.Tasks = tasks
	g.Progress = 0
	if len(tasks) > 0 {
		g.Progress = done * 100 / len(tasks)
	}
	switch {
	case len(tasks) > 0 && done == len(tasks):
		g.Status = domain.GoalDone
	case done > 0:
		g.Status = domain.GoalInProgress
	default:
		g.Status = domain.GoalAgreed
	}
	return g
}

func (a *App) stepDoneMap(ctx context.Context, internID string) map[string]bool {
	out := map[string]bool{}
	if states, err := a.store.StepStates(ctx, internID); err == nil {
		for _, s := range states {
			out[s.StepID] = s.Status == domain.StepDone
		}
	}
	return out
}

func (a *App) goalTasks(ctx context.Context, intern domain.User, in []TaskInput) ([]domain.GoalTask, error) {
	tpl, _ := a.templateFor(ctx, intern)
	var out []domain.GoalTask
	for _, t := range in {
		t.Title = strings.TrimSpace(t.Title)
		if t.StepID != "" {
			st, ok := tpl.Step(t.StepID)
			if !ok {
				return nil, invalid("Шаг плана не найден")
			}
			if t.Title == "" {
				t.Title = st.Title
			}
		}
		if n := utf8.RuneCountInString(t.Title); n < 3 || n > 200 {
			return nil, invalid("Задача — от 3 до 200 символов")
		}
		out = append(out, domain.GoalTask{ID: newID("gt"), Title: t.Title, StepID: t.StepID})
	}
	if len(out) == 0 {
		return nil, invalid("Добавьте хотя бы одну задачу — по ним считается прогресс цели")
	}
	if len(out) > 30 {
		return nil, invalid("Не больше 30 задач в цели")
	}
	return out, nil
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
	goals, err := a.store.Goals(ctx, internID)
	if err != nil {
		return nil, err
	}
	done := a.stepDoneMap(ctx, internID)
	for i := range goals {
		goals[i] = withProgress(goals[i], done)
	}
	return goals, nil
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
	tasks, err := a.goalTasks(ctx, intern, in.Tasks)
	if err != nil {
		return domain.Goal{}, err
	}
	now := a.now()
	g := domain.Goal{ID: newID("g"), InternID: intern.ID, Title: in.Title, Metric: in.Metric, Due: in.Due,
		Status: domain.GoalAgreed, Tasks: tasks, CreatedBy: actor.ID, CreatedAt: a.stamp(), UpdatedAt: now}
	if err := a.store.SaveGoal(ctx, g); err != nil {
		return g, err
	}
	g = withProgress(g, a.stepDoneMap(ctx, intern.ID))
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

// UpdateGoal — комментарий к цели.
func (a *App) UpdateGoal(ctx context.Context, actor domain.User, id string, in GoalUpdate) (domain.Goal, error) {
	g, intern, err := a.goalFor(ctx, actor, id)
	if err != nil {
		return g, err
	}
	if in.Note != nil {
		if utf8.RuneCountInString(*in.Note) > 500 {
			return g, invalid("Комментарий длиннее 500 символов")
		}
		g.Note = strings.TrimSpace(*in.Note)
	}
	return a.saveGoal(ctx, intern, g, actor)
}

func (a *App) goalFor(ctx context.Context, actor domain.User, id string) (domain.Goal, domain.User, error) {
	g, err := a.store.Goal(ctx, id)
	if err != nil {
		return g, domain.User{}, ErrNotFound
	}
	intern, ok := a.userByID(ctx, g.InternID)
	if !ok {
		return g, intern, ErrNotFound
	}
	if actor.ID != intern.ID && !canSetGoals(actor, intern) {
		return g, intern, ErrForbidden
	}
	return g, intern, nil
}

func (a *App) saveGoal(ctx context.Context, intern domain.User, g domain.Goal, actor domain.User) (domain.Goal, error) {
	wasDone := withProgress(g, a.stepDoneMap(ctx, intern.ID)).Status == domain.GoalDone
	g.UpdatedAt = a.now()
	if err := a.store.SaveGoal(ctx, g); err != nil {
		return g, err
	}
	g = withProgress(g, a.stepDoneMap(ctx, intern.ID))
	a.hub.Publish(intern.ID, events.Update{Type: "goal", Data: g})
	if g.Status == domain.GoalDone && !wasDone {
		a.Track(ctx, intern.ID, "goal_done", nil)
	}
	_ = actor
	return g, nil
}

// SetGoalTask — отметить свою задачу цели (задачи-шаги закрываются в плане).
func (a *App) SetGoalTask(ctx context.Context, actor domain.User, goalID, taskID string, done bool) (domain.Goal, error) {
	g, intern, err := a.goalFor(ctx, actor, goalID)
	if err != nil {
		return g, err
	}
	before := withProgress(g, a.stepDoneMap(ctx, intern.ID)).Status
	found := false
	for i := range g.Tasks {
		if g.Tasks[i].ID == taskID {
			if g.Tasks[i].StepID != "" {
				return g, invalid("Эта задача — шаг плана: отметьте его в плане")
			}
			g.Tasks[i].Done, found = done, true
		}
	}
	if !found {
		return g, ErrNotFound
	}
	out, err := a.saveGoal(ctx, intern, g, actor)
	if err == nil && out.Status == domain.GoalDone && before != domain.GoalDone && actor.ID == intern.ID && intern.LeadID != "" {
		a.notify(ctx, intern.LeadID, "plan", fmt.Sprintf("%s выполнил(а) цель: %s", intern.Name, g.Title), "/team")
	}
	return out, err
}

// AddGoalTask — руководитель или наставник добавляет задачу в цель.
func (a *App) AddGoalTask(ctx context.Context, actor domain.User, goalID string, in TaskInput) (domain.Goal, error) {
	g, intern, err := a.goalFor(ctx, actor, goalID)
	if err != nil {
		return g, err
	}
	if !canSetGoals(actor, intern) {
		return g, ErrForbidden
	}
	t, err := a.goalTasks(ctx, intern, []TaskInput{in})
	if err != nil {
		return g, err
	}
	g.Tasks = append(g.Tasks, t...)
	return a.saveGoal(ctx, intern, g, actor)
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
			var tasks []domain.GoalTask
			for _, t := range in.Tasks {
				tasks = append(tasks, domain.GoalTask{ID: newID("gt"), Title: t.Title, StepID: t.StepID})
			}
			g := domain.Goal{ID: newID("g"), InternID: intern, Title: in.Title, Metric: in.Metric, Due: in.Due,
				Status: domain.GoalAgreed, Tasks: tasks, CreatedBy: by[intern], CreatedAt: a.stamp(), UpdatedAt: a.now()}
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
