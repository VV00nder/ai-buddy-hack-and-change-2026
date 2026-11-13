package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"aibuddy/internal/domain"
	"aibuddy/internal/kv"
	"aibuddy/internal/mcp"
)

// Регистрация сотрудников и перевод между отделами. Учётные записи и отделы
// живут в корпоративном справочнике (SSO), AI-buddy только вызывает его.

// TeamInfo — отдел и его руководитель.
type TeamInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	LeadID   string `json:"leadId"`
	LeadName string `json:"leadName"`
}

// RegisterInput — данные регистрации: ФИО, почта и пароль. Отдел и роль назначает руководитель.
type RegisterInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Teams — отделы с руководителями (для регистрации и перевода).
func (a *App) Teams(ctx context.Context) ([]TeamInfo, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.CorpBaseURL+"/sso/teams", nil)
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()
	var out struct {
		Teams []TeamInfo `json:"teams"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&out) != nil {
		return nil, ErrUnavailable
	}
	return out.Teams, nil
}

// Register создаёт учётную запись в справочнике и сразу открывает сессию.
// client — адрес клиента для ограничения частоты регистраций.
func (a *App) Register(ctx context.Context, in RegisterInput, client string) (string, domain.User, error) {
	if ok, err := a.kv.Allow(ctx, "register:"+client, 5, time.Hour); err == nil && !ok {
		return "", domain.User{}, ErrRateLimited
	}
	body, _ := json.Marshal(in)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.CorpBaseURL+"/sso/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return "", domain.User{}, ErrUnavailable
	}
	defer resp.Body.Close()
	var out struct {
		Token   string      `json:"token"`
		User    domain.User `json:"user"`
		Message string      `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	switch {
	case resp.StatusCode == http.StatusBadRequest:
		return "", domain.User{}, invalid("%s", out.Message)
	case resp.StatusCode != http.StatusCreated || out.Token == "":
		return "", domain.User{}, ErrUnavailable
	}
	a.invalidateUsers()
	sid := newID("s") + newID("")
	if err := a.kv.SetSession(ctx, sid, kv.Session{User: out.User, Token: out.Token}, a.cfg.SessionTTL); err != nil {
		return "", domain.User{}, err
	}
	a.Track(ctx, out.User.ID, "registered", nil)
	a.notify(ctx, out.User.ID, "welcome", "Учётная запись создана. Руководитель примет вас в отдел и назначит роль — после этого откроются план и инструменты.", "/")
	if users, err := a.DemoUsers(ctx); err == nil {
		for _, u := range users {
			if u.Role == domain.RoleLead {
				a.notify(ctx, u.ID, "team", fmt.Sprintf("Новый сотрудник ждёт назначения в отдел: %s (%s)", out.User.Name, out.User.Email), "/rights")
			}
		}
	}
	return sid, out.User, nil
}

func roleRu(r domain.Role) string {
	switch r {
	case domain.RoleIntern:
		return "стажёр"
	case domain.RoleMentor:
		return "наставник"
	case domain.RoleLead:
		return "руководитель"
	case domain.RoleHRBP:
		return "HR BP"
	case domain.RoleNewcomer:
		return "ожидает назначения"
	}
	return "сотрудник"
}

// Department — сотрудники отдела руководителя, отделы для перевода и новые
// сотрудники, ждущие назначения.
type Department struct {
	Members   []domain.User `json:"members"`
	Teams     []TeamInfo    `json:"teams"`
	Newcomers []domain.User `json:"newcomers"`
	Mentors   []domain.User `json:"mentors"`
}

// MyDepartment — сотрудники отдела (видит руководитель).
func (a *App) MyDepartment(ctx context.Context, s kv.Session) (Department, error) {
	if s.User.Role != domain.RoleLead {
		return Department{}, ErrForbidden
	}
	users, err := a.DemoUsers(ctx)
	if err != nil {
		return Department{}, err
	}
	teams, err := a.Teams(ctx)
	if err != nil {
		return Department{}, err
	}
	d := Department{Members: []domain.User{}, Teams: teams, Newcomers: []domain.User{}, Mentors: []domain.User{}}
	for _, u := range users {
		if u.Team == s.User.Team && u.ID != s.User.ID {
			d.Members = append(d.Members, u)
			if u.Role == domain.RoleMentor {
				d.Mentors = append(d.Mentors, u)
			}
		}
		if u.Role == domain.RoleNewcomer {
			d.Newcomers = append(d.Newcomers, u)
		}
	}
	return d, nil
}

// Transfer — руководитель переводит сотрудника своего отдела в другой отдел
// с привязкой к руководителю нового отдела.
func (a *App) Transfer(ctx context.Context, s kv.Session, userID, team string) (Department, error) {
	if s.User.Role != domain.RoleLead {
		return Department{}, ErrForbidden
	}
	res, err := a.corp.Call(ctx, s.Token, "directory_transfer", map[string]any{"user_id": userID, "team": team})
	var ce *mcp.CallError
	if errors.As(err, &ce) {
		if ce.Code == "forbidden" {
			return Department{}, ErrForbidden
		}
		return Department{}, invalid("%s", ce.Message)
	}
	if err != nil {
		return Department{}, ErrUnavailable
	}
	a.invalidateUsers()
	u, ok := a.userByID(ctx, userID)
	if !ok {
		return Department{}, ErrNotFound
	}
	leadName := ""
	if l, ok := res["lead"].(map[string]any); ok {
		leadName = fmt.Sprint(l["name"])
	}
	msg := fmt.Sprintf("%s перевёл(а) вас в отдел «%s». Ваш руководитель — %s", s.User.Name, u.TeamName, leadName)
	if m, ok := res["mentor"].(map[string]any); ok {
		msg += fmt.Sprintf(", наставник — %v", m["name"])
	}
	a.notify(ctx, u.ID, "team", msg, "/")
	a.notify(ctx, u.LeadID, "team", fmt.Sprintf("В ваш отдел переведён сотрудник: %s (%s), перевёл(а) %s", u.Name, roleRu(u.Role), s.User.Name), "/team")
	if u.MentorID != "" && u.MentorID != u.LeadID {
		a.notify(ctx, u.MentorID, "team", "Вы — наставник стажёра, переведённого в ваш отдел: "+u.Name, "/team")
	}
	a.Track(ctx, s.User.ID, "employee_transferred", map[string]any{"user": u.ID, "team": u.Team})
	if u.Role == domain.RoleIntern {
		a.publishPlan(ctx, u)
	}
	return a.MyDepartment(ctx, s)
}

// AssignNewcomer — руководитель принимает нового сотрудника в свой отдел и назначает роль.
func (a *App) AssignNewcomer(ctx context.Context, s kv.Session, userID, role, mentorID string) (Department, error) {
	if s.User.Role != domain.RoleLead {
		return Department{}, ErrForbidden
	}
	res, err := a.corp.Call(ctx, s.Token, "directory_assign", map[string]any{"user_id": userID, "role": role, "mentor_id": mentorID})
	var ce *mcp.CallError
	if errors.As(err, &ce) {
		if ce.Code == "forbidden" {
			return Department{}, ErrForbidden
		}
		return Department{}, invalid("%s", ce.Message)
	}
	if err != nil {
		return Department{}, ErrUnavailable
	}
	a.invalidateUsers()
	u, ok := a.userByID(ctx, userID)
	if !ok {
		return Department{}, ErrNotFound
	}
	msg := fmt.Sprintf("%s принял(а) вас в отдел «%s», роль — %s", s.User.Name, u.TeamName, roleRu(u.Role))
	if m, ok := res["mentor"].(map[string]any); ok {
		msg += fmt.Sprintf(", наставник — %v", m["name"])
		a.notify(ctx, u.MentorID, "team", "Вы — наставник нового стажёра: "+u.Name, "/team")
	}
	a.notify(ctx, u.ID, "team", msg, "/")
	a.Track(ctx, s.User.ID, "newcomer_assigned", map[string]any{"user": u.ID, "role": u.Role})
	return a.MyDepartment(ctx, s)
}
