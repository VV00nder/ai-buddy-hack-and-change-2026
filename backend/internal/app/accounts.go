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

// RegisterInput — данные регистрации.
type RegisterInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Team     string `json:"team"`
	Role     string `json:"role"`
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
	a.Track(ctx, out.User.ID, "registered", map[string]any{"role": out.User.Role, "team": out.User.Team})
	a.notify(ctx, out.User.ID, "welcome", "Добро пожаловать в AI-buddy! Ваш руководитель увидит вас в своём отделе.", "/")
	if out.User.LeadID != "" {
		a.notify(ctx, out.User.LeadID, "team", fmt.Sprintf("В ваш отдел зарегистрировался сотрудник: %s (%s)", out.User.Name, roleRu(out.User.Role)), "/rights")
	}
	if out.User.MentorID != "" {
		a.notify(ctx, out.User.MentorID, "team", "Вы — наставник нового стажёра: "+out.User.Name, "/team")
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
	}
	return "сотрудник"
}

// Department — сотрудники отдела руководителя и отделы, куда можно перевести.
type Department struct {
	Members []domain.User `json:"members"`
	Teams   []TeamInfo    `json:"teams"`
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
	d := Department{Members: []domain.User{}, Teams: teams}
	for _, u := range users {
		if u.Team == s.User.Team && u.ID != s.User.ID {
			d.Members = append(d.Members, u)
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
