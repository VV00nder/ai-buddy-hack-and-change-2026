package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aibuddy/internal/domain"
	"aibuddy/internal/events"
)

// Проактивные напоминания. В Авито их запускает внутренний ИИ-крон
// (POST /api/cron/reminders); в прототипе — ещё и встроенный таймер.

// staleAfter — через сколько заявка или вопрос считаются «зависшими».
const staleAfter = 24 * time.Hour

// ReminderReport — что сделал прогон.
type ReminderReport struct {
	Digests       int `json:"digests"`
	StaleRequests int `json:"staleRequests"`
	OpenQuestions int `json:"openQuestions"`
	Reminders     int `json:"reminders"`
}

// once отмечает, что напоминание key уже отправлено сегодня.
func (a *App) once(key string) bool {
	a.cronMu.Lock()
	defer a.cronMu.Unlock()
	day := a.now().Format("2006-01-02")
	if a.cronSent[key] == day {
		return false
	}
	a.cronSent[key] = day
	return true
}

// RunReminders — один прогон ИИ-крона.
func (a *App) RunReminders(ctx context.Context) (ReminderReport, error) {
	var rep ReminderReport
	if !a.ff().Proactive {
		return rep, ErrDisabled
	}
	rep.Reminders = a.FireReminders(ctx)
	users, err := a.DemoUsers(ctx)
	if err != nil {
		return rep, err
	}
	for _, u := range users {
		if u.Role != domain.RoleIntern {
			continue
		}
		if a.digest(ctx, u) {
			rep.Digests++
		}
		acts, err := a.store.Actions(ctx, u.ID)
		if err != nil {
			return rep, err
		}
		for _, act := range acts {
			if act.Status != domain.ActionSubmitted || act.Kind == domain.ActionMentorQuestion || a.now().Sub(act.UpdatedAt) < staleAfter {
				continue
			}
			approver, _ := act.Result["approver_id"].(string)
			if approver == "" {
				approver, _ = act.Preview["approver_id"].(string)
			}
			if approver == "" || !a.once("stale:"+act.ID) {
				continue
			}
			days := int(a.now().Sub(act.UpdatedAt).Hours() / 24)
			a.notify(ctx, approver, "approval", fmt.Sprintf("%s ждёт решения %d дн.: %s", u.Name, days, actionLabel(act)), "/approvals")
			a.Track(ctx, approver, "reminder_sent", map[string]any{"kind": "stale_request"})
			rep.StaleRequests++
		}
		qs, err := a.store.QuestionsOfIntern(ctx, u.ID)
		if err != nil {
			return rep, err
		}
		for _, q := range qs {
			if q.Status != "open" || a.now().Sub(q.CreatedAt) < staleAfter || !a.once("question:"+q.ID) {
				continue
			}
			a.notify(ctx, q.MentorID, "question", fmt.Sprintf("Вопрос от %s ждёт ответа больше суток: «%s»", u.Name, q.Text), "/inbox")
			a.Track(ctx, q.MentorID, "reminder_sent", map[string]any{"kind": "open_question"})
			rep.OpenQuestions++
		}
	}
	return rep, nil
}

// digest — утреннее сообщение стажёру в чат: день, этап, следующий шаг, просрочки.
func (a *App) digest(ctx context.Context, u domain.User) bool {
	p, err := a.PlanFor(ctx, u)
	if err != nil || p.Next == nil || !a.once("digest:"+u.ID) {
		return false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Доброе утро, %s! Сегодня %d-й день стажировки.", domain.FirstName(u.Name), p.Day)
	for _, per := range p.Periods {
		if per.ID == p.CurrentPeriod && p.Day-1 == per.FromDay {
			fmt.Fprintf(&b, " Начинается этап «%s» — в нём %d шагов.", per.Name, per.Total)
		}
	}
	if p.Overdue > 0 {
		fmt.Fprintf(&b, "\n\nПросрочено шагов: **%d**. Давай закроем их первыми — если что-то мешает, отметь шаг как «Есть блокер», и я подскажу наставнику.", p.Overdue)
	}
	fmt.Fprintf(&b, "\n\n**Следующий шаг:** %s\n%s", p.Next.Title, p.Next.Description)
	msg := domain.Message{ID: newID("m"), UserID: u.ID, Author: "assistant", Status: domain.MsgDone, Mode: "proactive",
		Text: b.String(), CreatedAt: a.stamp(), UpdatedAt: a.now()}
	if p.Next.Prompt != "" {
		msg.Suggestions = []string{p.Next.Prompt}
	}
	if err := a.store.SaveMessage(ctx, msg); err != nil {
		return false
	}
	a.hub.Publish(u.ID, events.Update{Type: "message", Data: msg})
	a.notify(ctx, u.ID, "plan", fmt.Sprintf("День %d: следующий шаг — %s", p.Day, p.Next.Title), "/chat")
	a.Track(ctx, u.ID, "reminder_sent", map[string]any{"kind": "digest", "overdue": p.Overdue})
	return true
}

// StartReminders запускает встроенный таймер (если внешний крон не настроен).
func (a *App) StartReminders(ctx context.Context, every time.Duration) {
	// напоминания, которые пользователи поставили сами, проверяются каждую минуту
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				a.FireReminders(ctx)
			}
		}
	}()
	if every <= 0 || !a.ff().Proactive {
		return
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if rep, err := a.RunReminders(ctx); err != nil {
					a.log.Error("reminders", "err", err)
				} else {
					a.log.Info("reminders", "digests", rep.Digests, "stale", rep.StaleRequests, "questions", rep.OpenQuestions)
				}
			}
		}
	}()
}
