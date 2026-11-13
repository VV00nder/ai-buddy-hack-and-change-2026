package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"aibuddy/internal/domain"
	"aibuddy/internal/events"
	"aibuddy/internal/kv"
	"aibuddy/internal/mcp"
)

// Approval — заявка, ожидающая решения руководителя.
type Approval struct {
	Kind       string         `json:"kind"` // absence | access
	ExternalID string         `json:"externalId"`
	UserID     string         `json:"userId"`
	UserName   string         `json:"userName"`
	Title      string         `json:"title"`
	Details    map[string]any `json:"details"`
}

// Approvals — входящие на согласование (руководитель).
func (a *App) Approvals(ctx context.Context, s kv.Session) ([]Approval, error) {
	if !canApprove(s.User) {
		return nil, ErrForbidden
	}
	out := []Approval{}
	abs, err := a.corp.Call(ctx, s.Token, "absence_list", map[string]any{"scope": "approvals"})
	if err != nil {
		return nil, ErrUnavailable
	}
	for _, it := range asList(abs["items"]) {
		m := it.(map[string]any)
		title := absenceRange(m)
		out = append(out, Approval{Kind: "absence", ExternalID: fmt.Sprint(m["id"]), UserID: fmt.Sprint(m["userId"]),
			UserName: fmt.Sprint(m["userName"]), Title: title, Details: m})
	}
	acc, err := a.corp.Call(ctx, s.Token, "access_list", map[string]any{"scope": "approvals"})
	if err != nil {
		return nil, ErrUnavailable
	}
	for _, it := range asList(acc["items"]) {
		m := it.(map[string]any)
		title := fmt.Sprintf("Доступ: %v", m["systemName"])
		if p, _ := m["project"].(string); p != "" {
			title += " — " + p
		}
		title += ", " + levelName[fmt.Sprint(m["level"])]
		out = append(out, Approval{Kind: "access", ExternalID: fmt.Sprint(m["id"]), UserID: fmt.Sprint(m["userId"]),
			UserName: fmt.Sprint(m["userName"]), Title: title, Details: m})
	}
	return out, nil
}

var levelName = map[string]string{"read": "чтение", "edit": "редактирование", "admin": "администрирование"}

// canApprove — согласующими бывают руководители, наставники (владельцы проектов),
// инженеры (DevOps) и заместители руководителя; конкретного согласующего проверяет сервис заявок.
func canApprove(u domain.User) bool {
	return u.Role != domain.RoleIntern
}

// absenceRange — «Отгул на 12.11 (чт), 1 раб. день» или «Отпуск 14.12–18.12, 5 раб. дней».
func absenceRange(m map[string]any) string {
	kind := "Отгул"
	if m["type"] == "vacation" {
		kind = "Отпуск"
	}
	from, to := shortDate(fmt.Sprint(m["dateFrom"])), shortDate(fmt.Sprint(m["dateTo"]))
	days := fmt.Sprint(m["days"])
	n, _ := strconv.Atoi(days)
	unit := "раб. дней"
	switch {
	case n%10 == 1 && n%100 != 11:
		unit = "раб. день"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		unit = "раб. дня"
	}
	if from == to {
		return fmt.Sprintf("%s на %s, %s %s", kind, from, days, unit)
	}
	return fmt.Sprintf("%s %s–%s, %s %s", kind, from, to, days, unit)
}

var shortWD = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

func shortDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("02.01") + " (" + shortWD[t.Weekday()] + ")"
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// Decide — решение руководителя по заявке; результат возвращается стажёру.
func (a *App) Decide(ctx context.Context, s kv.Session, kind, externalID string, approve bool, comment string) error {
	if !canApprove(s.User) {
		return ErrForbidden
	}
	if utf8.RuneCountInString(comment) > 500 {
		return invalid("Комментарий длиннее 500 символов")
	}
	tool, decision := "", ""
	switch kind {
	case "absence":
		tool, decision = "absence_decide", map[bool]string{true: "approved", false: "rejected"}[approve]
	case "access":
		tool, decision = "access_decide", map[bool]string{true: "granted", false: "rejected"}[approve]
	default:
		return invalid("Неизвестный тип заявки")
	}
	res, err := a.corp.Call(ctx, s.Token, tool, map[string]any{"id": externalID, "decision": decision, "comment": strings.TrimSpace(comment)})
	var ce *mcp.CallError
	if errors.As(err, &ce) {
		if ce.Code == "forbidden" {
			return ErrForbidden
		}
		return invalid("%s", ce.Message)
	}
	if err != nil {
		return ErrUnavailable
	}
	a.Track(ctx, s.User.ID, "approval_decided", map[string]any{"kind": kind, "approved": approve})
	act, err := a.store.ActionByExternalID(ctx, externalID)
	if err != nil {
		return nil // заявка создана не через AI-buddy — уведомлять некого
	}
	act.Result = res
	act.Status = domain.ActionRejected
	if approve {
		act.Status = domain.ActionApproved
	}
	act.UpdatedAt = a.now()
	if err := a.store.SaveAction(ctx, act); err != nil {
		return err
	}
	a.hub.Publish(act.UserID, events.Update{Type: "action", Data: act})
	verdict := "Согласовано"
	if !approve {
		verdict = "Отклонено"
	}
	text := fmt.Sprintf("%s: %s (%s)", verdict, actionLabel(act), s.User.Name)
	if c := strings.TrimSpace(comment); c != "" {
		text += " — «" + c + "»"
	}
	a.notify(ctx, act.UserID, "action", text, "/actions/"+act.ID)
	if approve {
		a.Track(ctx, act.UserID, "action_completed", map[string]any{"kind": act.Kind})
		if intern, ok := a.userByID(ctx, act.UserID); ok {
			a.autoCompleteSteps(ctx, intern)
		}
	}
	return nil
}

// Inbox — вопросы стажёров наставнику.
func (a *App) Inbox(ctx context.Context, u domain.User) ([]domain.Question, error) {
	if u.Role != domain.RoleMentor {
		return nil, ErrForbidden
	}
	qs, err := a.store.QuestionsForMentor(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return qs, nil
}

// MyQuestions — вопросы стажёра наставнику.
func (a *App) MyQuestions(ctx context.Context, u domain.User) ([]domain.Question, error) {
	return a.store.QuestionsOfIntern(ctx, u.ID)
}

// AnswerQuestion — наставник отвечает; ответ приходит стажёру в чат и,
// по желанию, сохраняется в базу знаний команды.
func (a *App) AnswerQuestion(ctx context.Context, u domain.User, qid, answer string, addToFAQ bool) (domain.Question, error) {
	q, err := a.store.Question(ctx, qid)
	if err != nil {
		return q, ErrNotFound
	}
	if q.MentorID != u.ID {
		return q, ErrForbidden
	}
	answer = strings.TrimSpace(answer)
	if utf8.RuneCountInString(answer) < 2 || utf8.RuneCountInString(answer) > 4000 {
		return q, invalid("Ответ должен быть от 2 до 4000 символов")
	}
	if q.Status == "answered" {
		return q, invalid("На вопрос уже ответили")
	}
	now := a.now()
	q.Answer, q.Status, q.AnsweredAt, q.AddToFAQ = answer, "answered", &now, addToFAQ && a.flags.FAQFromAnswers
	if err := a.store.SaveQuestion(ctx, q); err != nil {
		return q, err
	}
	msg := domain.Message{ID: newID("m"), UserID: q.InternID, Author: "assistant", Status: domain.MsgDone, Mode: "mentor",
		Text: fmt.Sprintf("**%s ответил на твой вопрос** «%s»:\n\n%s", u.Name, q.Text, answer), CreatedAt: a.stamp(), UpdatedAt: now}
	_ = a.store.SaveMessage(ctx, msg)
	a.hub.Publish(q.InternID, events.Update{Type: "message", Data: msg})
	a.notify(ctx, q.InternID, "question", u.Name+" ответил на вопрос", "/chat")
	if act, err := a.store.ActionByExternalID(ctx, q.ID); err == nil {
		act.Status, act.UpdatedAt = domain.ActionApproved, now
		act.Result = map[string]any{"id": q.ID, "status": "answered", "answer": answer}
		_ = a.store.SaveAction(ctx, act)
		a.hub.Publish(q.InternID, events.Update{Type: "action", Data: act})
	}
	if q.AddToFAQ {
		f := domain.FAQEntry{ID: q.ID, Team: u.Team, Question: q.Text, Answer: answer, AuthorID: u.ID, CreatedAt: now}
		if err := a.store.AddFAQ(ctx, f); err == nil {
			a.kb.AddFAQ(f)
		}
	}
	a.Track(ctx, u.ID, "mentor_answered", map[string]any{"faq": q.AddToFAQ})
	return q, nil
}

// LoadFAQ поднимает сохранённые ответы наставников в базу знаний при старте.
func (a *App) LoadFAQ(ctx context.Context) error {
	faqs, err := a.store.FAQs(ctx)
	if err != nil {
		return err
	}
	for _, f := range faqs {
		a.kb.AddFAQ(f)
	}
	return nil
}

// InternSummary — строка обзора команды.
type InternSummary struct {
	User          domain.User `json:"user"`
	Day           int         `json:"day"`
	Percent       int         `json:"percent"`
	Done          int         `json:"done"`
	Total         int         `json:"total"`
	Overdue       int         `json:"overdue"`
	NextStep      string      `json:"nextStep,omitempty"`
	OpenQuestions int         `json:"openQuestions"`
	GoalsDone     int         `json:"goalsDone"`
	GoalsTotal    int         `json:"goalsTotal"`
	TemplateName  string      `json:"templateName"`
	Customized    bool        `json:"customized"`
	QuizzesPassed int         `json:"quizzesPassed"`
	QuizzesFailed int         `json:"quizzesFailed"` // тесты, которые пока не сданы ни разу
}

// Team — стажёры, которых видит наставник, руководитель или HR BP.
func (a *App) Team(ctx context.Context, viewer domain.User) ([]InternSummary, error) {
	if viewer.Role == domain.RoleIntern {
		return nil, ErrForbidden
	}
	users, err := a.DemoUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := []InternSummary{}
	for _, u := range users {
		if u.Role != domain.RoleIntern || !canViewIntern(viewer, u) {
			continue
		}
		p, err := a.PlanFor(ctx, u)
		if err != nil {
			return nil, err
		}
		qs, _ := a.store.QuestionsOfIntern(ctx, u.ID)
		open := 0
		for _, q := range qs {
			if q.Status == "open" {
				open++
			}
		}
		s := InternSummary{User: u, Day: p.Day, Percent: p.Percent, Done: p.Done, Total: p.Total, Overdue: p.Overdue, OpenQuestions: open}
		goals, _ := a.store.Goals(ctx, u.ID)
		for _, g := range goals {
			s.GoalsTotal++
			if g.Status == domain.GoalDone {
				s.GoalsDone++
			}
		}
		if p.Next != nil {
			s.NextStep = p.Next.Title
		}
		s.TemplateName, s.Customized = p.TemplateName, p.Customized
		atts, _ := a.attemptsOf(ctx, u.ID)
		passed, tried := map[string]bool{}, map[string]bool{}
		for _, at := range atts {
			tried[at.QuizID] = true
			passed[at.QuizID] = passed[at.QuizID] || at.Passed
		}
		for id := range tried {
			if passed[id] {
				s.QuizzesPassed++
			} else {
				s.QuizzesFailed++
			}
		}
		out = append(out, s)
	}
	return out, nil
}
