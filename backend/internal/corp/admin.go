package corp

import (
	"crypto/rand"
	"slices"

	"aibuddy/internal/domain"
	"aibuddy/internal/identity"
	"aibuddy/internal/mcp"
)

// Администратор сервиса стоит над руководителями: назначает любому сотруднику
// роль и отдел, принимает новичков в любой отдел, согласует любые заявки,
// сбрасывает пароли.

func (s *Service) isAdmin(id string) bool {
	p, ok := s.users[id]
	return ok && p.Role == domain.RoleAdmin
}

// manager — руководитель или администратор (под s.mu).
func (s *Service) manager(c identity.Claims) (*Person, bool) {
	p, ok := s.users[c.Sub]
	return p, ok && (p.Role == domain.RoleLead || p.Role == domain.RoleAdmin)
}

// assignableRoles — какие роли может назначить администратор.
var assignableRoles = []domain.Role{domain.RoleIntern, domain.RoleEngineer, domain.RoleMentor, domain.RoleLead, domain.RoleHRBP}

// SetUser — администратор меняет роль и отдел сотрудника. Если у отдела меняется
// руководитель, сотрудники отдела и ждущие решения заявки переходят к новому.
func (s *Service) SetUser(c identity.Claims, userID, role, teamID, mentorID string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	if !s.isAdmin(c.Sub) {
		return nil, mcp.ErrForbidden
	}
	p, ok := s.users[userID]
	switch {
	case !ok:
		return nil, mcp.NewToolError("not_found", "Сотрудник не найден")
	case p.Role == domain.RoleAdmin:
		return nil, mcp.NewToolError("not_allowed_for_role", "Роль администратора меняется только в конфигурации сервиса")
	}
	r := domain.Role(role)
	if !slices.Contains(assignableRoles, r) {
		return nil, mcp.NewToolError("bad_role", "Роль: стажёр, сотрудник, наставник, руководитель или HR BP")
	}
	team, ok := s.teams[teamID]
	if !ok {
		return nil, mcp.NewToolError("bad_team", "Выберите отдел из списка")
	}
	if r == domain.RoleLead {
		if l := s.leadOf(team.ID); l != nil && l.ID != p.ID {
			return nil, mcp.NewToolError("lead_exists", "В отделе «"+team.Name+"» уже есть руководитель: "+l.Name+". Сначала назначьте ему другую роль")
		}
	}
	mentor := ""
	if r == domain.RoleIntern {
		if m, ok := s.users[mentorID]; mentorID != "" && ok && m.Team == team.ID && m.Role == domain.RoleMentor {
			mentor = m.ID
		} else if mentorID != "" {
			return nil, mcp.NewToolError("bad_mentor", "Наставник должен быть из выбранного отдела")
		} else if m := s.mentorOf(team.ID); m != nil {
			mentor = m.ID
		}
	}
	oldTeam, wasNew := p.Team, p.Role == domain.RoleNewcomer
	p.Role, p.Team, p.TeamName = r, team.ID, team.Name
	p.MentorID, p.MentorIDs = mentor, nil
	if wasNew {
		p.StartDate = s.today()
	}
	if oldTeam != team.ID {
		for lead := range s.deputies {
			delete(s.deputies[lead], p.ID)
		}
	}
	s.relink(oldTeam)
	s.relink(team.ID)
	s.persist()
	out := map[string]any{"user": s.users[p.ID].User}
	if l := s.users[p.LeadID]; l != nil {
		out["lead"] = map[string]any{"id": l.ID, "name": l.Name}
	}
	if m := s.users[p.MentorID]; m != nil {
		out["mentor"] = map[string]any{"id": m.ID, "name": m.Name}
	}
	return out, nil
}

// relink привязывает сотрудников отдела к его текущему руководителю, стажёров
// без наставника — к наставнику отдела, и переносит ждущие решения заявки.
func (s *Service) relink(teamID string) {
	if teamID == "" {
		return
	}
	lead := s.leadOf(teamID)
	newLead := ""
	if lead != nil {
		newLead = lead.ID
	}
	for _, p := range s.users {
		if p.Team != teamID {
			continue
		}
		if p.Role == domain.RoleLead {
			p.LeadID = ""
			continue
		}
		if p.Role == domain.RoleIntern {
			// остаются только наставники, которые всё ещё наставники этого отдела
			var keep []string
			for _, id := range p.Mentors() {
				if m := s.users[id]; m != nil && m.Role == domain.RoleMentor && m.Team == teamID {
					keep = append(keep, id)
				}
			}
			if len(keep) == 0 {
				if m := s.mentorOf(teamID); m != nil {
					keep = []string{m.ID}
				}
			}
			p.MentorID, p.MentorIDs = "", nil
			if len(keep) > 0 {
				p.MentorID = keep[0]
			}
			if len(keep) > 1 {
				p.MentorIDs = keep
			}
		} else {
			p.MentorID, p.MentorIDs = "", nil
		}
		// пока у отдела нет руководителя, прежняя привязка и заявки остаются — до назначения нового
		if lead == nil || p.LeadID == newLead {
			continue
		}
		old := p.LeadID
		p.LeadID = newLead
		for _, a := range s.absences {
			if a.UserID == p.ID && a.Status == "pending" && a.ApproverID == old {
				a.ApproverID, a.Approver = lead.ID, lead.Name
			}
		}
		for _, r := range s.access {
			if r.UserID == p.ID && r.Status == "pending" && r.ApproverID == old && old != "" {
				r.ApproverID, r.Approver = lead.ID, lead.Name
			}
		}
	}
}

// ResetPassword — администратор задаёт сотруднику новый пароль.
func (s *Service) ResetPassword(c identity.Claims, userID, password string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	if !s.isAdmin(c.Sub) {
		return nil, mcp.ErrForbidden
	}
	p, ok := s.users[userID]
	if !ok {
		return nil, mcp.NewToolError("not_found", "Сотрудник не найден")
	}
	if !validPassword(password) {
		return nil, mcp.NewToolError("weak_password", "Пароль: не короче 8 символов, буквы и цифры")
	}
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	s.creds[p.ID] = cred{Salt: salt, Hash: hashPassword(password, salt)}
	s.persist()
	return map[string]any{"user": p.User, "reset": true}, nil
}

// DeleteRequest — руководитель сотрудника или администратор удаляет заявку
// (на отсутствие или доступ) из системы заявок.
func (s *Service) DeleteRequest(c identity.Claims, kind, id string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	may := func(userID string) bool {
		p, ok := s.users[userID]
		return s.isAdmin(c.Sub) || (ok && p.LeadID == c.Sub && s.users[c.Sub] != nil && s.users[c.Sub].Role == domain.RoleLead)
	}
	switch kind {
	case "absence":
		for i, a := range s.absences {
			if a.ID == id {
				if !may(a.UserID) {
					return nil, mcp.ErrForbidden
				}
				s.absences = slices.Delete(s.absences, i, i+1)
				return map[string]any{"id": id, "deleted": true}, nil
			}
		}
	case "access":
		for i, r := range s.access {
			if r.ID == id {
				if !may(r.UserID) {
					return nil, mcp.ErrForbidden
				}
				s.access = slices.Delete(s.access, i, i+1)
				return map[string]any{"id": id, "deleted": true}, nil
			}
		}
	default:
		return nil, mcp.NewToolError("bad_kind", "kind: absence | access")
	}
	return nil, mcp.NewToolError("not_found", "Заявка не найдена")
}

// SetMentors — руководитель стажёра или администратор назначает стажёру наставников
// (одного или нескольких); первый — основной.
func (s *Service) SetMentors(c identity.Claims, userID string, mentorIDs []string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	p, ok := s.users[userID]
	if !ok || p.Role != domain.RoleIntern {
		return nil, mcp.NewToolError("not_found", "Стажёр не найден")
	}
	if !s.isAdmin(c.Sub) && !(p.LeadID == c.Sub && s.users[c.Sub] != nil && s.users[c.Sub].Role == domain.RoleLead) {
		return nil, mcp.ErrForbidden
	}
	var ids []string
	for _, id := range mentorIDs {
		m, ok := s.users[id]
		if !ok || m.Role != domain.RoleMentor {
			return nil, mcp.NewToolError("bad_mentor", "Наставником можно назначить только сотрудника с ролью «Наставник»")
		}
		if m.Team != p.Team && !s.isAdmin(c.Sub) {
			return nil, mcp.NewToolError("bad_mentor", "Наставник должен быть из отдела стажёра")
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, mcp.NewToolError("mentor_required", "У стажёра должен быть хотя бы один наставник")
	}
	if len(ids) > 5 {
		return nil, mcp.NewToolError("too_many", "Не больше 5 наставников")
	}
	p.MentorID, p.MentorIDs = ids[0], nil
	if len(ids) > 1 {
		p.MentorIDs = ids
	}
	s.persist()
	names := []map[string]any{}
	for _, id := range ids {
		names = append(names, map[string]any{"id": id, "name": s.users[id].Name})
	}
	return map[string]any{"user": p.User, "mentors": names}, nil
}
