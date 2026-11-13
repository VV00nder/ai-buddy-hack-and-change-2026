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
	return a.teams(ctx, false)
}

// teams — отделы; all — в том числе без руководителя.
func (a *App) teams(ctx context.Context, all bool) ([]TeamInfo, error) {
	url := a.cfg.CorpBaseURL + "/sso/teams"
	if all {
		url += "?all=1"
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
			if u.Role == domain.RoleLead || u.Role == domain.RoleAdmin {
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
	case domain.RoleAdmin:
		return "администратор"
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

// MyDepartment — сотрудники отдела (видит руководитель; администратор — всех сотрудников).
func (a *App) MyDepartment(ctx context.Context, s kv.Session) (Department, error) {
	if s.User.Role != domain.RoleLead && !isAdmin(s.User) {
		return Department{}, ErrForbidden
	}
	users, err := a.DemoUsers(ctx)
	if err != nil {
		return Department{}, err
	}
	teams, err := a.teams(ctx, isAdmin(s.User))
	if err != nil {
		return Department{}, err
	}
	d := Department{Members: []domain.User{}, Teams: teams, Newcomers: []domain.User{}, Mentors: []domain.User{}}
	for _, u := range users {
		if u.Role != domain.RoleNewcomer && sameTeam(s.User, u.Team) && u.ID != s.User.ID {
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
	if s.User.Role != domain.RoleLead && !isAdmin(s.User) {
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
// Администратор выбирает и отдел (team), руководитель принимает только в свой.
func (a *App) AssignNewcomer(ctx context.Context, s kv.Session, userID, role, mentorID, team string) (Department, error) {
	if s.User.Role != domain.RoleLead && !isAdmin(s.User) {
		return Department{}, ErrForbidden
	}
	res, err := a.corp.Call(ctx, s.Token, "directory_assign", map[string]any{"user_id": userID, "role": role, "mentor_id": mentorID, "team": team})
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

// SetUser — администратор назначает сотруднику роль и отдел.
func (a *App) SetUser(ctx context.Context, s kv.Session, userID, role, team, mentorID string) (Department, error) {
	if !isAdmin(s.User) {
		return Department{}, ErrForbidden
	}
	before, _ := a.userByID(ctx, userID)
	res, err := a.corp.Call(ctx, s.Token, "admin_set_user", map[string]any{"user_id": userID, "role": role, "team": team, "mentor_id": mentorID})
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
	msg := fmt.Sprintf("%s назначил(а) вам роль «%s» в отделе «%s»", s.User.Name, roleRu(u.Role), u.TeamName)
	if l, ok := res["lead"].(map[string]any); ok && u.Role != domain.RoleLead {
		msg += fmt.Sprintf(", руководитель — %v", l["name"])
	}
	if m, ok := res["mentor"].(map[string]any); ok {
		msg += fmt.Sprintf(", наставник — %v", m["name"])
	}
	a.notify(ctx, u.ID, "team", msg, "/")
	a.Track(ctx, s.User.ID, "user_role_changed", map[string]any{"user": u.ID, "from": before.Role, "role": u.Role, "team": u.Team})
	if u.Role == domain.RoleIntern {
		a.publishPlan(ctx, u)
	}
	return a.MyDepartment(ctx, s)
}

// ResetPassword — администратор задаёт сотруднику новый пароль.
func (a *App) ResetPassword(ctx context.Context, s kv.Session, userID, password string) error {
	if !isAdmin(s.User) {
		return ErrForbidden
	}
	_, err := a.corp.Call(ctx, s.Token, "admin_reset_password", map[string]any{"user_id": userID, "password": password})
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
	a.notify(ctx, userID, "team", "Администратор задал вам новый пароль — используйте его при следующем входе", "/")
	a.Track(ctx, s.User.ID, "password_reset", map[string]any{"user": userID})
	return nil
}

// CanAnswer — может ли пользователь отвечать на вопросы стажёра.
func (a *App) CanAnswer(ctx context.Context, u domain.User, internID string) bool {
	intern, ok := a.userByID(ctx, internID)
	return ok && canAnswer(u, intern)
}

// SetMentors — руководитель стажёра или администратор назначает стажёру наставников.
func (a *App) SetMentors(ctx context.Context, s kv.Session, internID string, mentorIDs []string) (domain.User, error) {
	before, ok := a.userByID(ctx, internID)
	if !ok || before.Role != domain.RoleIntern {
		return domain.User{}, ErrNotFound
	}
	if !canManageIntern(s.User, before) {
		return domain.User{}, ErrForbidden
	}
	_, err := a.corp.Call(ctx, s.Token, "directory_set_mentors", map[string]any{"user_id": internID, "mentor_ids": mentorIDs})
	var ce *mcp.CallError
	if errors.As(err, &ce) {
		if ce.Code == "forbidden" {
			return domain.User{}, ErrForbidden
		}
		return domain.User{}, invalid("%s", ce.Message)
	}
	if err != nil {
		return domain.User{}, ErrUnavailable
	}
	a.invalidateUsers()
	u, ok := a.userByID(ctx, internID)
	if !ok {
		return u, ErrNotFound
	}
	a.notify(ctx, u.ID, "team", fmt.Sprintf("%s назначил(а) вам наставников: %s", s.User.Name, a.mentorNames(ctx, u)), "/")
	for _, id := range u.Mentors() {
		if !before.HasMentor(id) {
			a.notify(ctx, id, "team", "Вы — наставник стажёра: "+u.Name, "/team")
		}
	}
	a.Track(ctx, s.User.ID, "mentors_changed", map[string]any{"user": u.ID, "count": len(u.Mentors())})
	a.publishPlan(ctx, u)
	return u, nil
}
