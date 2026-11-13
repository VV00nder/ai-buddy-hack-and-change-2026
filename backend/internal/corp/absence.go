package corp

import (
	"fmt"
	"strings"
	"time"

	"aibuddy/internal/domain"
	"aibuddy/internal/identity"
	"aibuddy/internal/mcp"
)

// Правила синтетической политики отсутствий (не регламент Авито).
const (
	dayOffNoticeWorkdays = 2
	dayOffMaxDays        = 3
	vacationNoticeDays   = 14
	vacationMinTenure    = 90
)

// AbsencePolicy — описание правил для ассистента.
func AbsencePolicy() map[string]any {
	return map[string]any{
		"disclaimer": "Тестовая политика для прототипа, не регламент Авито.",
		"types": []map[string]any{
			{"type": "day_off", "name": "Отгул", "rules": []string{
				fmt.Sprintf("Подать заявку минимум за %d рабочих дня до даты отгула", dayOffNoticeWorkdays),
				fmt.Sprintf("Не больше %d рабочих дней подряд", dayOffMaxDays),
				"Списывается из баланса отгулов (у стажёров — 3 дня на программу)",
				"Согласует руководитель команды",
				"После согласования предупреди наставника и перенеси встречи в календаре",
			}},
			{"type": "vacation", "name": "Отпуск", "rules": []string{
				fmt.Sprintf("Подать заявку минимум за %d календарных дней", vacationNoticeDays),
				fmt.Sprintf("Доступен после %d дней работы в компании", vacationMinTenure),
				"Согласует руководитель команды",
			}},
		},
		"outOfScope": "Больничные и финансовые вопросы AI-buddy не оформляет: их ведёт HR BP.",
	}
}

// AbsenceInput — параметры заявки.
type AbsenceInput struct {
	Type     string `json:"type"`
	DateFrom string `json:"date_from"`
	DateTo   string `json:"date_to"`
	Reason   string `json:"reason"`
	DryRun   bool   `json:"dry_run"`
}

// CreateAbsence проверяет заявку и (если не dry_run) создаёт её.
func (s *Service) CreateAbsence(c identity.Claims, in AbsenceInput) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	p, ok := s.users[c.Sub]
	if !ok {
		return nil, mcp.NewToolError("unknown_user", "Сотрудник не найден в справочнике")
	}
	if in.Type == "" {
		in.Type = "day_off"
	}
	if in.Type != "day_off" && in.Type != "vacation" {
		return nil, mcp.NewToolError("bad_type", "Поддерживаются типы day_off (отгул) и vacation (отпуск)")
	}
	from, err1 := time.ParseInLocation(dateLayout, in.DateFrom, s.loc)
	if in.DateTo == "" {
		in.DateTo = in.DateFrom
	}
	to, err2 := time.ParseInLocation(dateLayout, in.DateTo, s.loc)
	if err1 != nil || err2 != nil {
		return nil, mcp.NewToolError("bad_dates", "Укажи даты в формате ГГГГ-ММ-ДД")
	}
	if to.Before(from) {
		return nil, mcp.NewToolError("bad_dates", "Дата окончания раньше даты начала")
	}
	today := s.today()
	if !from.After(today) {
		return nil, mcp.NewToolError("past_date", "Заявку можно подать только на будущие даты")
	}
	days := workdaysBetween(from, to)
	if days == 0 {
		return nil, mcp.NewToolError("weekend", "Выбранные даты приходятся на выходные — отгул не нужен")
	}
	lead := s.users[p.LeadID]
	if lead == nil {
		return nil, mcp.NewToolError("no_approver", "У сотрудника не указан руководитель")
	}
	if in.Type == "day_off" {
		earliest := addWorkdays(today, dayOffNoticeWorkdays)
		if from.Before(earliest) {
			return nil, mcp.NewToolError("too_late", fmt.Sprintf(
				"По правилам отгул оформляется минимум за %d рабочих дня. Ближайшая дата — %s. Если ситуация срочная, напиши руководителю (%s) напрямую.",
				dayOffNoticeWorkdays, humanDate(earliest), lead.Name))
		}
		if days > dayOffMaxDays {
			return nil, mcp.NewToolError("too_long", fmt.Sprintf("Отгул — не больше %d рабочих дней подряд. Для более долгого отсутствия оформи отпуск.", dayOffMaxDays))
		}
		if days > s.balance[p.ID] {
			return nil, mcp.NewToolError("no_balance", fmt.Sprintf("Недостаточно дней отгула: доступно %d, нужно %d", s.balance[p.ID], days))
		}
	} else {
		if p.Role == domain.RoleIntern || today.Sub(p.StartDate) < vacationMinTenure*24*time.Hour {
			return nil, mcp.NewToolError("not_eligible", fmt.Sprintf("Отпуск доступен после %d дней работы. Сейчас можно оформить отгул.", vacationMinTenure))
		}
		if from.Before(today.AddDate(0, 0, vacationNoticeDays)) {
			return nil, mcp.NewToolError("too_late", fmt.Sprintf("Отпуск оформляется минимум за %d дней", vacationNoticeDays))
		}
	}
	for _, a := range s.absences {
		if a.UserID == p.ID && a.Status != "rejected" && a.DateFrom <= in.DateTo && in.DateFrom <= a.DateTo {
			return nil, mcp.NewToolError("overlap", "На эти даты уже есть заявка "+a.ID)
		}
	}
	a := &Absence{UserID: p.ID, UserName: p.Name, Type: in.Type, DateFrom: in.DateFrom, DateTo: in.DateTo, Days: days,
		Reason: strings.TrimSpace(in.Reason), Status: "pending", ApproverID: lead.ID, Approver: lead.Name, CreatedAt: s.now()}
	out := map[string]any{
		"dry_run": in.DryRun, "type": a.Type, "date_from": a.DateFrom, "date_to": a.DateTo,
		"days": days, "approver": lead.Name, "approver_id": lead.ID, "balance_after": s.balance[p.ID] - days*boolInt(in.Type == "day_off"),
		"period":     humanDate(from) + map[bool]string{true: "", false: " — " + humanDate(to)}[from.Equal(to)],
		"next_steps": []string{"Руководитель согласует заявку", "Предупреди наставника и команду в чате", "Перенеси встречи на эти даты"},
	}
	if in.DryRun {
		out["status"] = "valid"
		return out, nil
	}
	a.ID = s.nextID("ABS")
	s.absences = append(s.absences, a)
	if a.Type == "day_off" {
		s.balance[p.ID] -= days
	}
	out["id"], out["status"] = a.ID, a.Status
	return out, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// AbsenceBalance — остаток отгулов.
func (s *Service) AbsenceBalance(c identity.Claims) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	return map[string]any{"day_off_left": s.balance[c.Sub]}, nil
}

// ListAbsences — свои заявки или заявки на согласование (для руководителя).
func (s *Service) ListAbsences(c identity.Claims, scope string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	items := []Absence{}
	for _, a := range s.absences {
		if (scope == "approvals" && s.mayDecide(a.ApproverID, c.Sub) && a.Status == "pending") || (scope != "approvals" && a.UserID == c.Sub) {
			items = append(items, *a)
		}
	}
	return map[string]any{"items": items}, nil
}

// GetAbsence — заявка по id (автор или согласующий).
func (s *Service) GetAbsence(c identity.Claims, id string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	for _, a := range s.absences {
		if a.ID == id {
			if a.UserID != c.Sub && !s.mayDecide(a.ApproverID, c.Sub) {
				return nil, mcp.ErrForbidden
			}
			return toMap(a), nil
		}
	}
	return nil, mcp.NewToolError("not_found", "Заявка не найдена")
}

// DecideAbsence — решение руководителя.
func (s *Service) DecideAbsence(c identity.Claims, id, decision, comment string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	if decision != "approved" && decision != "rejected" {
		return nil, mcp.NewToolError("bad_decision", "decision: approved | rejected")
	}
	for _, a := range s.absences {
		if a.ID != id {
			continue
		}
		if !s.mayDecide(a.ApproverID, c.Sub) {
			return nil, mcp.ErrForbidden
		}
		if a.Status != "pending" {
			return nil, mcp.NewToolError("already_decided", "Решение по заявке уже принято")
		}
		now := s.now()
		a.Status, a.Comment, a.DecidedAt, a.DecidedBy = decision, comment, &now, c.Sub
		if decision == "rejected" && a.Type == "day_off" {
			s.balance[a.UserID] += a.Days
		}
		return toMap(a), nil
	}
	return nil, mcp.NewToolError("not_found", "Заявка не найдена")
}
