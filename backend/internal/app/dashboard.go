package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"aibuddy/internal/domain"
	"aibuddy/internal/kv"
)

// Кабинет руководителя и наставника: всё на одной странице — что ждёт решения,
// кто отстаёт от плана, заявки стажёров и напоминания.

// InternCard — стажёр в кабинете: сводка для всплывающей подсказки и отметки.
type InternCard struct {
	InternSummary
	MentorName string `json:"mentorName"`
	// PendingApprovals — заявки стажёра, которые ждут решения этого пользователя.
	PendingApprovals int `json:"pendingApprovals"`
	// WaitingHours — сколько часов ждёт самая старая из них.
	WaitingHours int `json:"waitingHours"`
	// OpenRequests — заявки стажёра без решения у любого согласующего.
	OpenRequests int `json:"openRequests"`
	// Alerts — короткие напоминания рядом с именем.
	Alerts []string `json:"alerts"`
}

// Dashboard — сводка кабинета.
type Dashboard struct {
	Interns   []InternCard `json:"interns"`
	Approvals []Approval   `json:"approvals"`
	Reminders []Reminder   `json:"reminders"`
	Newcomers int          `json:"newcomers"`
}

func waitingHours(m map[string]any, now time.Time) int {
	t, err := time.Parse(time.RFC3339Nano, fmt.Sprint(m["createdAt"]))
	if err != nil {
		return 0
	}
	return int(now.Sub(t).Hours())
}

// Dashboard — стажёры с отметками, заявки на решение и напоминания.
func (a *App) Dashboard(ctx context.Context, s kv.Session) (Dashboard, error) {
	u := s.User
	if u.Role == domain.RoleIntern || u.Role == domain.RoleNewcomer {
		return Dashboard{}, ErrForbidden
	}
	team, err := a.Team(ctx, u)
	if err != nil {
		return Dashboard{}, err
	}
	d := Dashboard{Interns: []InternCard{}, Approvals: []Approval{}, Reminders: []Reminder{}}
	if canApprove(u) {
		if ap, err := a.Approvals(ctx, s); err == nil {
			d.Approvals = ap
		}
	}
	now := a.now()
	for i := range d.Approvals {
		d.Approvals[i].WaitingHours = waitingHours(d.Approvals[i].Details, now)
	}
	sort.SliceStable(d.Approvals, func(i, j int) bool { return d.Approvals[i].WaitingHours > d.Approvals[j].WaitingHours })
	for _, r := range team {
		c := InternCard{InternSummary: r, Alerts: []string{}}
		if m, ok := a.userByID(ctx, r.User.MentorID); ok {
			c.MentorName = m.Name
		}
		for _, ap := range d.Approvals {
			if ap.UserID == r.User.ID {
				c.PendingApprovals++
				c.WaitingHours = max(c.WaitingHours, ap.WaitingHours)
			}
		}
		if acts, err := a.store.Actions(ctx, r.User.ID); err == nil {
			for _, ac := range acts {
				if ac.Status == domain.ActionSubmitted && ac.Kind != domain.ActionMentorQuestion {
					c.OpenRequests++
				}
			}
		}
		if c.Overdue > 0 {
			c.Alerts = append(c.Alerts, fmt.Sprintf("отстаёт от плана: просрочено %d", c.Overdue))
		}
		if c.PendingApprovals > 0 {
			msg := fmt.Sprintf("ждёт вашего решения: %d", c.PendingApprovals)
			if time.Duration(c.WaitingHours)*time.Hour >= staleAfter {
				msg += fmt.Sprintf(" (уже %s)", humanHours(c.WaitingHours))
			}
			c.Alerts = append(c.Alerts, msg)
		}
		if c.OpenQuestions > 0 {
			c.Alerts = append(c.Alerts, fmt.Sprintf("вопросов без ответа: %d", c.OpenQuestions))
		}
		if c.QuizzesFailed > 0 {
			c.Alerts = append(c.Alerts, fmt.Sprintf("не сдан тест: %d", c.QuizzesFailed))
		}
		d.Interns = append(d.Interns, c)
	}
	// сначала те, кому нужно внимание
	sort.SliceStable(d.Interns, func(i, j int) bool { return len(d.Interns[i].Alerts) > len(d.Interns[j].Alerts) })
	if rs, err := a.Reminders(ctx, u); err == nil {
		d.Reminders = rs
	}
	if u.Role == domain.RoleLead {
		if users, err := a.DemoUsers(ctx); err == nil {
			for _, x := range users {
				if x.Role == domain.RoleNewcomer {
					d.Newcomers++
				}
			}
		}
	}
	return d, nil
}

func humanHours(h int) string {
	if h < 48 {
		return fmt.Sprintf("%d ч", h)
	}
	return fmt.Sprintf("%d дн", h/24)
}

// InternRequest — заявка стажёра в его карточке.
type InternRequest struct {
	ID         string              `json:"id"`
	Kind       domain.ActionKind   `json:"kind"`
	Title      string              `json:"title"`
	Status     domain.ActionStatus `json:"status"`
	ExternalID string              `json:"externalId,omitempty"`
	Comment    string              `json:"comment,omitempty"`
	CreatedAt  time.Time           `json:"createdAt"`
	// AwaitingMe — решение по заявке ждёт текущего пользователя.
	AwaitingMe bool `json:"awaitingMe"`
}

// InternRequests — какие заявки отправлял стажёр и какие ещё не согласованы.
func (a *App) InternRequests(ctx context.Context, s kv.Session, internID string) ([]InternRequest, error) {
	intern, ok := a.userByID(ctx, internID)
	if !ok || intern.Role != domain.RoleIntern {
		return nil, ErrNotFound
	}
	if !canViewIntern(s.User, intern) {
		return nil, ErrForbidden
	}
	acts, err := a.store.Actions(ctx, internID)
	if err != nil {
		return nil, err
	}
	mine := map[string]bool{}
	if canApprove(s.User) {
		if ap, err := a.Approvals(ctx, s); err == nil {
			for _, x := range ap {
				mine[x.ExternalID] = true
			}
		}
	}
	out := []InternRequest{}
	for _, ac := range acts {
		// вопросы наставнику — не согласования, они в разделе вопросов
		if ac.Status == domain.ActionProposed || ac.Status == domain.ActionCancelled || ac.Kind == domain.ActionMentorQuestion {
			continue
		}
		out = append(out, InternRequest{ID: ac.ID, Kind: ac.Kind, Title: ac.Title, Status: ac.Status, ExternalID: ac.ExternalID,
			Comment: comment(ac), CreatedAt: ac.CreatedAt, AwaitingMe: ac.ExternalID != "" && mine[ac.ExternalID]})
	}
	return out, nil
}

// --- Напоминания ---

// Reminder — напоминание пользователю (например, «решить по заявке завтра в 10:00»).
type Reminder struct {
	ID      string    `json:"id"`
	OwnerID string    `json:"ownerId"`
	Text    string    `json:"text"`
	Link    string    `json:"link,omitempty"`
	At      time.Time `json:"at"`
	Sent    bool      `json:"sent"`
	Created time.Time `json:"created"`
}

// ReminderInput — текст и время: через N часов или в конкретный момент.
type ReminderInput struct {
	Text    string `json:"text"`
	Link    string `json:"link"`
	InHours int    `json:"inHours"`
	At      string `json:"at"` // RFC3339 или ГГГГ-ММ-ДДTЧЧ:ММ
}

// AddReminder создаёт напоминание.
func (a *App) AddReminder(ctx context.Context, u domain.User, in ReminderInput) (Reminder, error) {
	in.Text = strings.TrimSpace(in.Text)
	if n := utf8.RuneCountInString(in.Text); n < 3 || n > 300 {
		return Reminder{}, invalid("Текст напоминания — от 3 до 300 символов")
	}
	now := a.now()
	var at time.Time
	switch {
	case in.InHours > 0 && in.InHours <= 24*30:
		at = now.Add(time.Duration(in.InHours) * time.Hour)
	case in.At != "":
		t, err := time.Parse(time.RFC3339, in.At)
		if err != nil {
			t, err = time.ParseInLocation("2006-01-02T15:04", in.At, now.Location())
		}
		if err != nil {
			return Reminder{}, invalid("Время напоминания в формате ГГГГ-ММ-ДД ЧЧ:ММ")
		}
		at = t
	default:
		return Reminder{}, invalid("Укажите, когда напомнить")
	}
	if !at.After(now) {
		return Reminder{}, invalid("Время напоминания уже прошло")
	}
	r := Reminder{ID: newID("rm"), OwnerID: u.ID, Text: in.Text, Link: in.Link, At: at, Created: now}
	if err := putRec(ctx, a.store, recReminder, r.ID, r); err != nil {
		return Reminder{}, err
	}
	a.Track(ctx, u.ID, "reminder_set", map[string]any{"inHours": int(at.Sub(now).Hours())})
	return r, nil
}

// Reminders — активные напоминания пользователя (ближайшие первыми).
func (a *App) Reminders(ctx context.Context, u domain.User) ([]Reminder, error) {
	all, err := listRec[Reminder](ctx, a.store, recReminder)
	if err != nil {
		return nil, err
	}
	out := []Reminder{}
	for _, r := range all {
		if r.OwnerID == u.ID && !r.Sent {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

// DeleteReminder удаляет своё напоминание.
func (a *App) DeleteReminder(ctx context.Context, u domain.User, id string) error {
	r, err := getRec[Reminder](ctx, a.store, recReminder, id)
	if err != nil {
		return err
	}
	if r.OwnerID != u.ID {
		return ErrNotFound
	}
	return a.store.DeleteRecord(ctx, recReminder, id)
}

// FireReminders отправляет наступившие напоминания (вызывается таймером и ИИ-кроном).
func (a *App) FireReminders(ctx context.Context) int {
	all, err := listRec[Reminder](ctx, a.store, recReminder)
	if err != nil {
		return 0
	}
	n := 0
	now := a.now()
	for _, r := range all {
		if r.Sent || r.At.After(now) {
			continue
		}
		r.Sent = true
		if putRec(ctx, a.store, recReminder, r.ID, r) == nil {
			a.notify(ctx, r.OwnerID, "reminder", "Напоминание: "+r.Text, r.Link)
			n++
		}
	}
	return n
}

func comment(ac domain.Action) string {
	if c, ok := ac.Result["comment"].(string); ok {
		return c
	}
	return ac.Error
}
