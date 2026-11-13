package corp

import (
	"sort"

	"aibuddy/internal/domain"
	"aibuddy/internal/identity"
	"aibuddy/internal/mcp"
)

// mayDecide — решение принимает сам согласующий или его заместитель.
// Вызывается под s.mu.
func (s *Service) mayDecide(approverID, sub string) bool {
	return approverID == sub || s.deputies[approverID][sub] || s.isAdmin(sub)
}

// SetDeputy — руководитель доверяет (или забирает) согласование своих заявок
// сотруднику своей команды. Стажёру право не передаётся.
func (s *Service) SetDeputy(c identity.Claims, userID string, enabled bool) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	lead, ok := s.users[c.Sub]
	if !ok || (lead.Role != domain.RoleLead && lead.Role != domain.RoleAdmin) {
		return nil, mcp.ErrForbidden
	}
	if lead.Role == domain.RoleAdmin { // администратор назначает заместителя руководителю отдела сотрудника
		u, ok := s.users[userID]
		if !ok || s.users[u.LeadID] == nil {
			return nil, mcp.NewToolError("not_found", "У сотрудника нет руководителя")
		}
		lead = s.users[u.LeadID]
	}
	u, ok := s.users[userID]
	if !ok || u.ID == lead.ID {
		return nil, mcp.NewToolError("not_found", "Сотрудник не найден")
	}
	if u.Team != lead.Team {
		return nil, mcp.NewToolError("other_team", "Передать согласование можно только сотруднику своей команды")
	}
	if u.Role == domain.RoleIntern {
		return nil, mcp.NewToolError("not_allowed_for_role", "Стажёр не может согласовывать заявки")
	}
	if s.deputies[lead.ID] == nil {
		s.deputies[lead.ID] = map[string]bool{}
	}
	if enabled {
		s.deputies[lead.ID][u.ID] = true
	} else {
		delete(s.deputies[lead.ID], u.ID)
	}
	return s.deputiesOf(lead.ID), nil
}

// Deputies — заместители руководителя по согласованию и те, чьи заявки может согласовать пользователь.
func (s *Service) Deputies(c identity.Claims) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	out := s.deputiesOf(c.Sub)
	if s.isAdmin(c.Sub) { // администратор видит заместителей всех руководителей
		all := []map[string]any{}
		for _, id := range s.sortedIDs() {
			if p := s.users[id]; p.Role == domain.RoleLead {
				all = append(all, asItems(s.deputiesOf(p.ID)["deputies"])...)
			}
		}
		out["deputies"] = all
	}
	var for_ []map[string]any
	for lead, ds := range s.deputies {
		if ds[c.Sub] {
			for_ = append(for_, map[string]any{"id": lead, "name": s.users[lead].Name})
		}
	}
	sort.Slice(for_, func(i, j int) bool { return for_[i]["id"].(string) < for_[j]["id"].(string) })
	out["actingFor"] = for_
	return out, nil
}

func (s *Service) deputiesOf(leadID string) map[string]any {
	items := []map[string]any{}
	for id := range s.deputies[leadID] {
		if u, ok := s.users[id]; ok {
			items = append(items, map[string]any{"id": u.ID, "name": u.Name, "role": u.Role})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["id"].(string) < items[j]["id"].(string) })
	return map[string]any{"deputies": items}
}

func asItems(v any) []map[string]any {
	l, _ := v.([]map[string]any)
	return l
}
