package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"aibuddy/internal/domain"
	"aibuddy/internal/kv"
	"aibuddy/internal/mcp"
)

// Права, которые руководитель передаёт сотрудникам своей команды.
const (
	// PermKB — добавлять и править инструкции, папки и тесты команды.
	PermKB = "kb"
	// PermApprovals — согласовывать заявки вместо руководителя (хранится в сервисе заявок).
	PermApprovals = "approvals"
	// PermPrograms — шаблоны обучения и программы стажёров (у руководителя и наставника по роли).
	PermPrograms = "programs"
)

// Grant — выданное право.
type Grant struct {
	UserID    string    `json:"userId"`
	Perm      string    `json:"perm"`
	Team      string    `json:"team"`
	GrantedBy string    `json:"grantedBy"`
	At        time.Time `json:"at"`
}

func grantID(userID, perm string) string { return userID + ":" + perm }

// hasGrant — право выдано руководителем.
func (a *App) hasGrant(ctx context.Context, u domain.User, perm string) bool {
	g, err := getRec[Grant](ctx, a.store, recGrant, grantID(u.ID, perm))
	return err == nil && g.Team == u.Team
}

// canEditKB — руководитель или сотрудник с правом на инструкции.
func (a *App) canEditKB(ctx context.Context, u domain.User) bool {
	return u.Role == domain.RoleLead || (u.Role != domain.RoleIntern && a.hasGrant(ctx, u, PermKB))
}

// canManagePrograms — шаблоны обучения ведут руководитель и наставники.
func canManagePrograms(u domain.User) bool {
	return u.Role == domain.RoleLead || u.Role == domain.RoleMentor
}

// Perms — права пользователя для интерфейса.
func (a *App) Perms(ctx context.Context, s kv.Session) []string {
	u := s.User
	out := []string{}
	if a.canEditKB(ctx, u) {
		out = append(out, PermKB)
	}
	if canManagePrograms(u) {
		out = append(out, PermPrograms)
	}
	switch {
	case u.Role == domain.RoleLead:
		out = append(out, PermApprovals)
	case u.Role != domain.RoleIntern:
		if res, err := a.corp.Call(ctx, s.Token, "delegation_list", nil); err == nil && len(asList(res["actingFor"])) > 0 {
			out = append(out, PermApprovals)
		}
	}
	return out
}

// Member — сотрудник команды и выданные ему права.
type Member struct {
	User  domain.User `json:"user"`
	Perms []string    `json:"perms"`
}

// Rights — права сотрудников команды (видит руководитель).
func (a *App) Rights(ctx context.Context, s kv.Session) ([]Member, error) {
	if s.User.Role != domain.RoleLead {
		return nil, ErrForbidden
	}
	users, err := a.DemoUsers(ctx)
	if err != nil {
		return nil, err
	}
	deputies := map[string]bool{}
	if res, err := a.corp.Call(ctx, s.Token, "delegation_list", nil); err == nil {
		for _, d := range asList(res["deputies"]) {
			if m, ok := d.(map[string]any); ok {
				deputies[fmt.Sprint(m["id"])] = true
			}
		}
	} else {
		return nil, ErrUnavailable
	}
	out := []Member{}
	for _, u := range users {
		if u.Team != s.User.Team || u.ID == s.User.ID || u.Role == domain.RoleIntern {
			continue
		}
		m := Member{User: u, Perms: []string{}}
		if a.hasGrant(ctx, u, PermKB) {
			m.Perms = append(m.Perms, PermKB)
		}
		if deputies[u.ID] {
			m.Perms = append(m.Perms, PermApprovals)
		}
		out = append(out, m)
	}
	return out, nil
}

// SetRight — руководитель выдаёт или забирает право у сотрудника своей команды.
func (a *App) SetRight(ctx context.Context, s kv.Session, userID, perm string, enabled bool) ([]Member, error) {
	lead := s.User
	if lead.Role != domain.RoleLead {
		return nil, ErrForbidden
	}
	u, ok := a.userByID(ctx, userID)
	if !ok || u.Team != lead.Team || u.ID == lead.ID {
		return nil, ErrNotFound
	}
	if u.Role == domain.RoleIntern {
		return nil, invalid("Стажёру права не передаются")
	}
	what := ""
	switch perm {
	case PermKB:
		what = "добавлять и править инструкции и тесты команды"
		if enabled {
			if err := putRec(ctx, a.store, recGrant, grantID(u.ID, perm), Grant{UserID: u.ID, Perm: perm, Team: u.Team, GrantedBy: lead.ID, At: a.now()}); err != nil {
				return nil, err
			}
		} else if err := a.store.DeleteRecord(ctx, recGrant, grantID(u.ID, perm)); err != nil {
			return nil, err
		}
	case PermApprovals:
		what = "согласовывать заявки вместо руководителя"
		_, err := a.corp.Call(ctx, s.Token, "delegation_set", map[string]any{"user_id": u.ID, "enabled": enabled})
		var ce *mcp.CallError
		if errors.As(err, &ce) {
			return nil, invalid("%s", ce.Message)
		}
		if err != nil {
			return nil, ErrUnavailable
		}
	default:
		return nil, invalid("Неизвестное право")
	}
	verb := "доверил(а) вам"
	if !enabled {
		verb = "забрал(а) у вас право"
	}
	a.notify(ctx, u.ID, "rights", fmt.Sprintf("%s %s %s", lead.Name, verb, what), "/")
	a.Track(ctx, lead.ID, "right_changed", map[string]any{"perm": perm, "enabled": enabled})
	return a.Rights(ctx, s)
}

func hasPerm(perms []string, p string) bool { return slices.Contains(perms, p) }
