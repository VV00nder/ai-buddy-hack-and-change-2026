package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"aibuddy/internal/domain"
	"aibuddy/internal/events"
	"aibuddy/internal/kv"
	"aibuddy/internal/mcp"
)

// Actions — действия пользователя (черновики и отправленные заявки).
func (a *App) Actions(ctx context.Context, u domain.User) ([]domain.Action, error) {
	return a.store.Actions(ctx, u.ID)
}

// Action — одно действие пользователя.
func (a *App) Action(ctx context.Context, u domain.User, id string) (domain.Action, error) {
	act, err := a.store.Action(ctx, id)
	if err != nil || act.UserID != u.ID {
		return domain.Action{}, ErrNotFound
	}
	return act, nil
}

// editable — поля, которые стажёр может поправить в карточке перед отправкой.
var editable = map[domain.ActionKind][]string{
	domain.ActionAbsence:        {"date_from", "date_to", "reason"},
	domain.ActionAccess:         {"level", "justification"},
	domain.ActionMentorQuestion: {"question"},
}

// ConfirmAction — стажёр подтверждает черновик; AI-buddy выполняет действие
// через корпоративный сервис (MCP), сохраняет и показывает результат.
func (a *App) ConfirmAction(ctx context.Context, s kv.Session, id string, edits map[string]any) (domain.Action, error) {
	act, err := a.Action(ctx, s.User, id)
	if err != nil {
		return act, err
	}
	if act.Status != domain.ActionProposed {
		return act, invalid("Действие уже выполнено или отменено")
	}
	for k, v := range edits {
		allowed := false
		for _, f := range editable[act.Kind] {
			if f == k {
				allowed = true
			}
		}
		sv, isStr := v.(string)
		if !allowed || !isStr || utf8.RuneCountInString(sv) > 1000 {
			return act, invalid("Поле %q нельзя изменить", k)
		}
		act.Args[k] = strings.TrimSpace(sv)
	}
	switch act.Kind {
	case domain.ActionMentorQuestion:
		return a.submitQuestion(ctx, s, act)
	case domain.ActionAbsence, domain.ActionAccess:
		return a.submitCorp(ctx, s, act)
	}
	return act, invalid("Неизвестный тип действия")
}

func (a *App) submitCorp(ctx context.Context, s kv.Session, act domain.Action) (domain.Action, error) {
	if (act.Kind == domain.ActionAbsence && !a.ff().AbsenceActions) || (act.Kind == domain.ActionAccess && !a.ff().AccessActions) {
		return act, ErrDisabled
	}
	res, err := a.corp.Call(ctx, s.Token, act.Tool, act.Args)
	var ce *mcp.CallError
	switch {
	case errors.As(err, &ce):
		// правила могли измениться (например, прошло время) — показываем причину
		act.Error = ce.Message
		act.Status = domain.ActionFailed
	case err != nil:
		// сервис недоступен: черновик сохраняется, можно повторить
		return act, ErrUnavailable
	default:
		act.Result = res
		act.ExternalID, _ = res["id"].(string)
		act.Status = domain.ActionSubmitted
		if st, _ := res["status"].(string); st == "granted" || st == "approved" {
			act.Status = domain.ActionApproved
		}
	}
	act.UpdatedAt = a.now()
	if err := a.store.SaveAction(ctx, act); err != nil {
		return act, err
	}
	a.hub.Publish(s.User.ID, events.Update{Type: "action", Data: act})
	if act.Status == domain.ActionFailed {
		a.Track(ctx, s.User.ID, "action_failed", map[string]any{"kind": act.Kind, "error": act.Error})
		return act, nil
	}
	a.Track(ctx, s.User.ID, "action_submitted", map[string]any{"kind": act.Kind, "status": act.Status})
	switch act.Status {
	case domain.ActionApproved:
		a.Track(ctx, s.User.ID, "action_completed", map[string]any{"kind": act.Kind})
		a.notify(ctx, s.User.ID, "action", "Доступ выдан: "+act.Title, "/actions/"+act.ID)
		a.autoCompleteSteps(ctx, s.User)
	case domain.ActionSubmitted:
		if lead := s.User.LeadID; lead != "" {
			a.notify(ctx, lead, "approval", fmt.Sprintf("%s просит: %s", s.User.Name, actionLabel(act)), "/approvals")
			a.hub.Publish(lead, events.Update{Type: "approval", Data: act})
		}
		a.notify(ctx, s.User.ID, "action", "Заявка отправлена на согласование: "+actionLabel(act), "/actions/"+act.ID)
	}
	return act, nil
}

func (a *App) submitQuestion(ctx context.Context, s kv.Session, act domain.Action) (domain.Action, error) {
	if !a.ff().MentorEscalation {
		return act, ErrDisabled
	}
	text, _ := act.Args["question"].(string)
	if utf8.RuneCountInString(strings.TrimSpace(text)) < 5 {
		return act, invalid("Вопрос слишком короткий")
	}
	if s.User.MentorID == "" {
		return act, invalid("У тебя не указан наставник")
	}
	q := domain.Question{ID: newID("q"), InternID: s.User.ID, MentorID: s.User.MentorID, Text: strings.TrimSpace(text),
		Status: "open", CreatedAt: a.now()}
	if err := a.store.SaveQuestion(ctx, q); err != nil {
		return act, err
	}
	act.Status, act.ExternalID, act.UpdatedAt = domain.ActionSubmitted, q.ID, a.now()
	act.Result = map[string]any{"id": q.ID, "status": "open"}
	if mentor, ok := a.userByID(ctx, s.User.MentorID); ok {
		act.Result["mentor"] = mentor.Name
	}
	if err := a.store.SaveAction(ctx, act); err != nil {
		return act, err
	}
	// вопрос видят все наставники стажёра (и руководитель — в «Стажёрах»)
	for _, id := range s.User.Mentors() {
		a.notify(ctx, id, "question", s.User.Name+" спрашивает: "+q.Text, "/inbox")
		a.hub.Publish(id, events.Update{Type: "question", Data: q})
	}
	a.hub.Publish(s.User.ID, events.Update{Type: "action", Data: act})
	a.Track(ctx, s.User.ID, "action_submitted", map[string]any{"kind": act.Kind})
	a.Track(ctx, s.User.ID, "mentor_escalation", nil)
	return act, nil
}

// actionLabel — название действия для уведомлений.
func actionLabel(act domain.Action) string {
	if act.Kind == domain.ActionAccess {
		return "доступ к " + act.Title
	}
	r := []rune(act.Title)
	if len(r) == 0 {
		return ""
	}
	return strings.ToLower(string(r[:1])) + string(r[1:])
}

// CancelAction — стажёр отказывается от черновика.
func (a *App) CancelAction(ctx context.Context, u domain.User, id string) (domain.Action, error) {
	act, err := a.Action(ctx, u, id)
	if err != nil {
		return act, err
	}
	if act.Status != domain.ActionProposed {
		return act, invalid("Отменить можно только неотправленный черновик")
	}
	act.Status, act.UpdatedAt = domain.ActionCancelled, a.now()
	if err := a.store.SaveAction(ctx, act); err != nil {
		return act, err
	}
	a.hub.Publish(u.ID, events.Update{Type: "action", Data: act})
	a.Track(ctx, u.ID, "action_cancelled", map[string]any{"kind": act.Kind})
	return act, nil
}
