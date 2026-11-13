package app

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"aibuddy/internal/agent"
	"aibuddy/internal/domain"
	"aibuddy/internal/events"
	"aibuddy/internal/kv"
)

// SendMessage сохраняет сообщение и запускает агента асинхронно: интерфейс
// сразу получает «ответ готовится» и не блокируется на время работы LLM.
func (a *App) SendMessage(ctx context.Context, s kv.Session, text string) (domain.Message, domain.Message, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domain.Message{}, domain.Message{}, invalid("Сообщение пустое")
	}
	if utf8.RuneCountInString(text) > a.cfg.MaxMessageLen {
		return domain.Message{}, domain.Message{}, invalid("Сообщение длиннее %d символов", a.cfg.MaxMessageLen)
	}
	ok, err := a.kv.Allow(ctx, "msg:"+s.User.ID, a.cfg.MsgRateLimit, time.Minute)
	if err != nil {
		return domain.Message{}, domain.Message{}, err
	}
	if !ok {
		return domain.Message{}, domain.Message{}, ErrRateLimited
	}
	history, err := a.store.Messages(ctx, s.User.ID, 12)
	if err != nil {
		return domain.Message{}, domain.Message{}, err
	}
	now := a.now()
	um := domain.Message{ID: newID("m"), UserID: s.User.ID, Author: "user", Text: text, Status: domain.MsgDone, CreatedAt: a.stamp(), UpdatedAt: now}
	am := domain.Message{ID: newID("m"), UserID: s.User.ID, Author: "assistant", Status: domain.MsgPending,
		CreatedAt: a.stamp(), UpdatedAt: now, Trace: []domain.TraceItem{{Kind: "thinking", Label: "Получил вопрос", At: now}}}
	if err := a.store.SaveMessage(ctx, um); err != nil {
		return um, am, err
	}
	if err := a.store.SaveMessage(ctx, am); err != nil {
		return um, am, err
	}
	a.wg.Add(1)
	go a.runAgent(s, text, history, am)
	return um, am, nil
}

func (a *App) runAgent(s kv.Session, text string, history []domain.Message, am domain.Message) {
	defer a.wg.Done()
	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.AgentTimeout)
	defer cancel()
	started := a.now()
	progress := func(t domain.TraceItem) {
		am.Trace = append(am.Trace, t)
		am.UpdatedAt = a.now()
		_ = a.store.SaveMessage(ctx, am)
		a.hub.Publish(s.User.ID, events.Update{Type: "message", Data: am})
	}
	reply := a.agent.Handle(ctx, agent.Request{User: s.User, Token: s.Token, Text: text, History: history, Progress: progress})
	reply = a.applyFlags(reply)
	am.Text, am.Sources, am.Suggestions, am.Mode, am.Context = reply.Text, reply.Sources, reply.Suggestions, reply.Mode, reply.Context
	am.QuizID = reply.QuizID
	am.Status, am.UpdatedAt = domain.MsgDone, a.now()
	if ctx.Err() != nil && am.Text == "" {
		am.Status, am.Text = domain.MsgError, "Ответ занял слишком много времени. Попробуй переформулировать вопрос или повтори позже."
	}
	if reply.Proposal != nil {
		act := domain.Action{ID: newID("a"), UserID: s.User.ID, Kind: reply.Proposal.Kind, Tool: reply.Proposal.Tool, Title: reply.Proposal.Title,
			Args: reply.Proposal.Args, Preview: reply.Proposal.Preview, Warnings: reply.Proposal.Warnings, Status: domain.ActionProposed,
			CreatedAt: a.now(), UpdatedAt: a.now()}
		if err := a.store.SaveAction(ctx, act); err == nil {
			am.ActionID = act.ID
			a.hub.Publish(s.User.ID, events.Update{Type: "action", Data: act})
			a.Track(ctx, s.User.ID, "action_proposed", map[string]any{"kind": act.Kind})
		}
	}
	if err := a.store.SaveMessage(ctx, am); err != nil {
		a.log.Error("save reply", "err", err)
	}
	elapsed := a.now().Sub(started)
	a.metrics.ObserveAgent(elapsed)
	a.Track(ctx, s.User.ID, "question_answered", map[string]any{"mode": reply.Mode, "answered": reply.Answered,
		"escalated": reply.Escalated, "seconds": elapsed.Seconds(), "sources": len(reply.Sources)})
	a.hub.Publish(s.User.ID, events.Update{Type: "message", Data: am})
}

// applyFlags убирает действия, отключённые feature flags.
func (a *App) applyFlags(r agent.Reply) agent.Reply {
	if r.Proposal == nil {
		return r
	}
	off := (r.Proposal.Kind == domain.ActionAbsence && !a.ff().AbsenceActions) ||
		(r.Proposal.Kind == domain.ActionAccess && !a.ff().AccessActions) ||
		(r.Proposal.Kind == domain.ActionMentorQuestion && !a.ff().MentorEscalation)
	if off {
		r.Proposal = nil
		r.Text += "\n\nОформление через AI-buddy сейчас отключено — воспользуйся корпоративным порталом."
	}
	return r
}

// Messages — история диалога.
func (a *App) Messages(ctx context.Context, u domain.User) ([]domain.Message, error) {
	return a.store.Messages(ctx, u.ID, 100)
}

// Feedback — оценка ответа («помогло / не помогло»).
func (a *App) Feedback(ctx context.Context, u domain.User, msgID string, helpful bool) error {
	m, err := a.store.Message(ctx, msgID)
	if err != nil || m.UserID != u.ID || m.Author != "assistant" {
		return ErrNotFound
	}
	m.Feedback = &helpful
	if err := a.store.SaveMessage(ctx, m); err != nil {
		return err
	}
	a.Track(ctx, u.ID, "feedback", map[string]any{"helpful": helpful, "mode": m.Mode})
	return nil
}
