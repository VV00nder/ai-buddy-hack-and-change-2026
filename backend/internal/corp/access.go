package corp

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"aibuddy/internal/identity"
	"aibuddy/internal/mcp"
)

var errOutage = errors.New("service unavailable")

func toMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

// Catalog — каталог систем с учётом роли (запрещённые помечены).
func (s *Service) Catalog(c identity.Claims) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	items := []map[string]any{}
	for _, sys := range s.catalog {
		m := toMap(sys)
		delete(m, "deniedRoles")
		delete(m, "deniedLevels")
		delete(m, "projects")
		if pr := sys.Projects[c.Team]; len(pr) > 0 {
			m["projects"] = pr
		}
		m["approverRole"] = s.approverRoles[sys.Approval]
		m["available"] = !slices.Contains(sys.DeniedRoles, string(c.Role))
		if st := s.accessStatus(c.Sub, sys.ID); st != "" {
			m["myStatus"] = st
		}
		items = append(items, m)
	}
	return map[string]any{"items": items}, nil
}

func (s *Service) accessStatus(userID, system string) string {
	st := ""
	for _, r := range s.access {
		if r.UserID == userID && r.System == system {
			st = r.Status
		}
	}
	return st
}

// AccessInput — параметры заявки на доступ.
type AccessInput struct {
	System        string `json:"system"`
	Project       string `json:"project"`
	Level         string `json:"level"`
	Justification string `json:"justification"`
	DryRun        bool   `json:"dry_run"`
}

// RequestAccess проверяет и создаёт заявку на доступ.
func (s *Service) RequestAccess(c identity.Claims, in AccessInput) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	p, ok := s.users[c.Sub]
	if !ok {
		return nil, mcp.NewToolError("unknown_user", "Сотрудник не найден в справочнике")
	}
	var sys *System
	for i := range s.catalog {
		if s.catalog[i].ID == in.System {
			sys = &s.catalog[i]
		}
	}
	if sys == nil {
		return nil, mcp.NewToolError("unknown_system", "Такой системы нет в каталоге доступов")
	}
	if slices.Contains(sys.DeniedRoles, string(p.Role)) {
		return nil, mcp.NewToolError("not_allowed_for_role", sys.DeniedHint)
	}
	if in.Level == "" {
		in.Level = sys.Levels[0]
	}
	if !slices.Contains(sys.Levels, in.Level) {
		return nil, mcp.NewToolError("bad_level", "Доступные уровни: "+levelsRu(sys.Levels))
	}
	if slices.Contains(sys.DeniedLevels[string(p.Role)], in.Level) {
		return nil, mcp.NewToolError("level_not_allowed", "Уровень «"+levelRu[in.Level]+"» для "+sys.Name+" стажёрам не выдаётся. Можно запросить: "+levelsRu(allowedLevels(sys, p.Role)))
	}
	in.Project = strings.TrimSpace(strings.ToLower(in.Project))
	if projects := sys.Projects[p.Team]; len(sys.Projects) > 0 {
		if in.Project == "" {
			return nil, mcp.NewToolError("project_required", "Укажи проект: "+strings.Join(projects, ", "))
		}
		if !slices.Contains(projects, in.Project) {
			return nil, mcp.NewToolError("unknown_project", "Проект не найден среди проектов твоей команды: "+strings.Join(projects, ", "))
		}
	} else {
		in.Project = ""
	}
	in.Justification = strings.TrimSpace(in.Justification)
	needJust := sys.JustificationRequired || in.Level == "admin"
	if needJust && utf8.RuneCountInString(in.Justification) < 15 {
		return nil, mcp.NewToolError("justification_required", "Для "+sys.Name+" нужно обоснование: для какой задачи нужен доступ (не короче 15 символов)")
	}
	for _, r := range s.access {
		if r.UserID == p.ID && r.System == sys.ID && r.Project == in.Project && (r.Status == "granted" || r.Status == "pending") {
			what := sys.Name
			if in.Project != "" {
				what += " (" + in.Project + ")"
			}
			return nil, mcp.NewToolError("duplicate", map[string]string{"granted": "Доступ к " + what + " уже выдан", "pending": "Заявка на " + what + " уже ждёт согласования"}[r.Status])
		}
	}
	r := &AccessRequest{UserID: p.ID, UserName: p.Name, System: sys.ID, SystemName: sys.Name, Level: in.Level, Project: in.Project,
		Justification: in.Justification, CreatedAt: s.now()}
	out := map[string]any{"dry_run": in.DryRun, "system": sys.ID, "system_name": sys.Name, "level": in.Level,
		"level_name": levelRu[in.Level], "approval": sys.Approval}
	if in.Project != "" {
		out["project"] = in.Project
	}
	if sys.Approval == "auto" {
		out["expected"] = "Доступ выдаётся автоматически сразу после отправки"
	} else {
		approver := s.approverFor(sys, p, in.Project)
		if approver == nil {
			return nil, mcp.NewToolError("no_approver", "Не удалось определить, кто согласует доступ")
		}
		role := s.approverRoles[sys.Approval]
		r.ApproverID, r.Approver, r.ApproverRole = approver.ID, approver.Name, role
		out["approver"], out["approver_role"], out["approver_id"] = approver.Name, role, approver.ID
		out["expected"] = "Заявку согласует " + role + ": " + approver.Name
	}
	if in.DryRun {
		out["status"] = "valid"
		return out, nil
	}
	r.ID = s.nextID("ACC")
	if sys.Approval == "auto" {
		now := s.now()
		r.Status, r.DecidedAt = "granted", &now
	} else {
		r.Status = "pending"
	}
	s.access = append(s.access, r)
	out["id"], out["status"] = r.ID, r.Status
	return out, nil
}

// approverFor — согласующий зависит от типа ресурса.
func (s *Service) approverFor(sys *System, p *Person, project string) *Person {
	switch sys.Approval {
	case "owner":
		if id, ok := s.owners[project]; ok {
			return s.users[id]
		}
		return s.users[p.LeadID]
	case "devops":
		return s.users[s.devops]
	}
	return s.users[p.LeadID]
}

var levelRu = map[string]string{"read": "чтение", "edit": "редактирование", "admin": "администрирование"}

func levelsRu(levels []string) string {
	out := make([]string, len(levels))
	for i, l := range levels {
		out[i] = levelRu[l] + " (" + l + ")"
	}
	return strings.Join(out, ", ")
}

func allowedLevels[R ~string](sys *System, role R) []string {
	var out []string
	for _, l := range sys.Levels {
		if !slices.Contains(sys.DeniedLevels[string(role)], l) {
			out = append(out, l)
		}
	}
	return out
}

// ListAccess — свои заявки или заявки на согласование.
func (s *Service) ListAccess(c identity.Claims, scope string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	items := []AccessRequest{}
	for _, r := range s.access {
		if (scope == "approvals" && s.mayDecide(r.ApproverID, c.Sub) && r.Status == "pending") || (scope != "approvals" && r.UserID == c.Sub) {
			items = append(items, *r)
		}
	}
	return map[string]any{"items": items}, nil
}

// DecideAccess — решение руководителя по заявке на доступ.
func (s *Service) DecideAccess(c identity.Claims, id, decision, comment string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	if decision != "granted" && decision != "rejected" {
		return nil, mcp.NewToolError("bad_decision", "decision: granted | rejected")
	}
	for _, r := range s.access {
		if r.ID != id {
			continue
		}
		if !s.mayDecide(r.ApproverID, c.Sub) {
			return nil, mcp.ErrForbidden
		}
		if r.Status != "pending" {
			return nil, mcp.NewToolError("already_decided", "Решение по заявке уже принято")
		}
		now := s.now()
		r.Status, r.Comment, r.DecidedAt, r.DecidedBy = decision, comment, &now, c.Sub
		return toMap(r), nil
	}
	return nil, mcp.NewToolError("not_found", "Заявка не найдена")
}

// WhoAmI — профиль и окружение сотрудника.
func (s *Service) WhoAmI(c identity.Claims) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	p, ok := s.users[c.Sub]
	if !ok {
		return nil, mcp.NewToolError("unknown_user", "Сотрудник не найден")
	}
	team := s.teams[p.Team]
	out := map[string]any{"profile": p.User, "handle": p.Handle, "team": team}
	if m := s.users[p.MentorID]; m != nil {
		out["mentor"] = map[string]any{"name": m.Name, "handle": m.Handle}
	}
	if l := s.users[p.LeadID]; l != nil {
		out["lead"] = map[string]any{"name": l.Name, "handle": l.Handle}
	}
	return out, nil
}

// TeamMembers — участники команды вызывающего (только своей команды).
func (s *Service) TeamMembers(c identity.Claims) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	members := []map[string]any{}
	for _, l := range loginOrder {
		p := s.byLogin[l]
		if p.Team == c.Team {
			members = append(members, map[string]any{"id": p.ID, "name": p.Name, "role": p.Role, "handle": p.Handle})
		}
	}
	return map[string]any{"team": s.teams[c.Team], "members": members}, nil
}
