package app

import (
	"context"
	"sort"

	"aibuddy/internal/domain"
	"aibuddy/internal/flags"
	"aibuddy/internal/kv"
)

// Администрирование сервиса: состояние, флаги возможностей, ИИ-крон и журнал действий.
// Доступно только администратору (роль admin).

const recFlags = "flags"

// ff — текущие флаги (администратор может менять их на лету).
func (a *App) ff() flags.Flags {
	a.flagsMu.RLock()
	defer a.flagsMu.RUnlock()
	return a.flags
}

// toggleable — флаги, которые можно менять без перезапуска (режим LLM задаётся при старте).
var toggleable = map[string]func(*flags.Flags) *bool{
	"absenceActions":   func(f *flags.Flags) *bool { return &f.AbsenceActions },
	"accessActions":    func(f *flags.Flags) *bool { return &f.AccessActions },
	"mentorEscalation": func(f *flags.Flags) *bool { return &f.MentorEscalation },
	"faqFromAnswers":   func(f *flags.Flags) *bool { return &f.FAQFromAnswers },
	"proactive":        func(f *flags.Flags) *bool { return &f.Proactive },
}

// LoadFlags применяет флаги, сохранённые администратором.
func (a *App) LoadFlags(ctx context.Context) error {
	saved, err := getRec[map[string]bool](ctx, a.store, recFlags, "current")
	if err != nil {
		return nil // ещё не меняли
	}
	a.flagsMu.Lock()
	defer a.flagsMu.Unlock()
	for name, v := range saved {
		if f, ok := toggleable[name]; ok {
			*f(&a.flags) = v
		}
	}
	return nil
}

// SetFlag включает или выключает возможность для всех пользователей.
func (a *App) SetFlag(ctx context.Context, s kv.Session, name string, enabled bool) (flags.Flags, error) {
	if !isAdmin(s.User) {
		return flags.Flags{}, ErrForbidden
	}
	f, ok := toggleable[name]
	if !ok {
		return flags.Flags{}, invalid("Этот флаг меняется только при запуске сервиса")
	}
	a.flagsMu.Lock()
	*f(&a.flags) = enabled
	cur := a.flags
	a.flagsMu.Unlock()
	saved := map[string]bool{}
	for n, get := range toggleable {
		saved[n] = *get(&cur)
	}
	if err := putRec(ctx, a.store, recFlags, "current", saved); err != nil {
		return cur, err
	}
	a.Track(ctx, s.User.ID, "flag_changed", map[string]any{"flag": name, "enabled": enabled})
	return cur, nil
}

// AuditEntry — запись журнала с именем пользователя.
type AuditEntry struct {
	domain.Event
	UserName string `json:"userName"`
}

// Service — сводка для администратора.
type Service struct {
	Status      map[string]string `json:"status"`
	Mode        string            `json:"mode"`
	Model       string            `json:"model,omitempty"`
	Flags       flags.Flags       `json:"flags"`
	Toggleable  []string          `json:"toggleable"`
	Roles       map[string]int    `json:"roles"`
	Pending     int               `json:"pending"`
	Newcomers   int               `json:"newcomers"`
	EventsTotal int               `json:"eventsTotal"`
	Audit       []AuditEntry      `json:"audit"`
}

// ServiceInfo — состояние сервиса, флаги, пользователи по ролям и журнал последних действий.
func (a *App) ServiceInfo(ctx context.Context, s kv.Session) (Service, error) {
	if !isAdmin(s.User) {
		return Service{}, ErrForbidden
	}
	out := Service{Status: a.Ready(ctx), Mode: a.Mode(), Flags: a.ff(), Roles: map[string]int{}, Audit: []AuditEntry{}}
	if out.Mode == "llm" {
		out.Model = a.agent.Model()
	}
	for n := range toggleable {
		out.Toggleable = append(out.Toggleable, n)
	}
	sort.Strings(out.Toggleable)
	names := map[string]string{}
	if users, err := a.DemoUsers(ctx); err == nil {
		for _, u := range users {
			out.Roles[string(u.Role)]++
			names[u.ID] = u.Name
			if u.Role == domain.RoleNewcomer {
				out.Newcomers++
			}
		}
	}
	if ap, err := a.Approvals(ctx, s); err == nil {
		out.Pending = len(ap)
	}
	evs, err := a.store.Events(ctx)
	if err != nil {
		return out, err
	}
	out.EventsTotal = len(evs)
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].At.After(evs[j].At) })
	for i, e := range evs {
		if i == 100 {
			break
		}
		out.Audit = append(out.Audit, AuditEntry{Event: e, UserName: names[e.UserID]})
	}
	return out, nil
}

// RunCronNow — администратор запускает ИИ-крон вручную (без служебного токена).
func (a *App) RunCronNow(ctx context.Context, s kv.Session) (ReminderReport, error) {
	if !isAdmin(s.User) {
		return ReminderReport{}, ErrForbidden
	}
	a.Track(ctx, s.User.ID, "cron_run", nil)
	return a.RunReminders(ctx)
}
