package corp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"aibuddy/internal/domain"
	"aibuddy/internal/identity"
	"aibuddy/internal/mcp"
)

func schema(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

func str(desc string, enum ...string) map[string]any {
	m := map[string]any{"type": "string", "description": desc}
	if len(enum) > 0 {
		m["enum"] = enum
	}
	return m
}

var dryRun = map[string]any{"type": "boolean", "description": "true — только проверить заявку без создания"}

type idArgs struct {
	ID       string `json:"id"`
	Scope    string `json:"scope"`
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, mcp.NewToolError("bad_args", "Некорректные аргументы инструмента")
	}
	return v, nil
}

func simple(f func(identity.Claims, idArgs) (map[string]any, error)) mcp.Handler {
	return func(_ context.Context, c identity.Claims, raw json.RawMessage) (map[string]any, error) {
		a, err := decode[idArgs](raw)
		if err != nil {
			return nil, err
		}
		return f(c, a)
	}
}

var (
	leads = []string{string(domain.RoleLead)}
	// approvers — роли, которые могут быть согласующими доступов; конкретного согласующего проверяет сервис.
	// Заместитель руководителя по согласованию может быть в любой роли, кроме стажёра.
	approvers = []string{string(domain.RoleLead), string(domain.RoleMentor), string(domain.RoleEngineer), string(domain.RoleHRBP)}
)

// MCPServer публикует корпоративные сервисы как MCP-инструменты.
func (s *Service) MCPServer(verify mcp.Verifier) *mcp.Server {
	srv := mcp.NewServer("avito-corp-mock", "1.0", verify)
	scope := str("mine — мои заявки, approvals — ждут моего решения", "mine", "approvals")
	tools := []mcp.Tool{
		{Info: mcp.ToolInfo{Name: "directory_whoami", Title: "Мой профиль", Description: "Профиль сотрудника: команда, наставник, руководитель, куратор, чат команды.",
			Annotations: map[string]any{"readOnlyHint": true}},
			Handler: simple(func(c identity.Claims, _ idArgs) (map[string]any, error) { return s.WhoAmI(c) })},
		{Info: mcp.ToolInfo{Name: "directory_team", Title: "Моя команда", Description: "Участники своей команды с ролями и контактами.",
			Annotations: map[string]any{"readOnlyHint": true}},
			Handler: simple(func(c identity.Claims, _ idArgs) (map[string]any, error) { return s.TeamMembers(c) })},
		{Info: mcp.ToolInfo{Name: "absence_policy", Title: "Правила отсутствий", Description: "Правила оформления отгула и отпуска.",
			Annotations: map[string]any{"readOnlyHint": true}},
			Handler: simple(func(identity.Claims, idArgs) (map[string]any, error) { return AbsencePolicy(), nil })},
		{Info: mcp.ToolInfo{Name: "absence_balance", Title: "Баланс отгулов", Description: "Сколько дней отгула осталось.",
			Annotations: map[string]any{"readOnlyHint": true}},
			Handler: simple(func(c identity.Claims, _ idArgs) (map[string]any, error) { return s.AbsenceBalance(c) })},
		{Info: mcp.ToolInfo{Name: "absence_create", Title: "Заявка на отсутствие",
			Description: "Создать заявку на отгул или отпуск. С dry_run=true только проверяет правила и возвращает предпросмотр.",
			InputSchema: schema(map[string]any{
				"type":      str("day_off — отгул, vacation — отпуск", "day_off", "vacation"),
				"date_from": str("Дата начала, ГГГГ-ММ-ДД"), "date_to": str("Дата окончания, ГГГГ-ММ-ДД"),
				"reason": str("Причина (необязательно)"), "dry_run": dryRun}, "date_from")},
			Handler: func(_ context.Context, c identity.Claims, raw json.RawMessage) (map[string]any, error) {
				in, err := decode[AbsenceInput](raw)
				if err != nil {
					return nil, err
				}
				return s.CreateAbsence(c, in)
			}},
		{Info: mcp.ToolInfo{Name: "absence_list", Title: "Мои заявки на отсутствие", Description: "Список заявок на отсутствие.",
			InputSchema: schema(map[string]any{"scope": scope}), Annotations: map[string]any{"readOnlyHint": true}},
			Handler: simple(func(c identity.Claims, a idArgs) (map[string]any, error) { return s.ListAbsences(c, a.Scope) })},
		{Info: mcp.ToolInfo{Name: "absence_get", Title: "Статус заявки", Description: "Заявка на отсутствие по идентификатору.",
			InputSchema: schema(map[string]any{"id": str("ID заявки")}, "id"), Annotations: map[string]any{"readOnlyHint": true}},
			Handler: simple(func(c identity.Claims, a idArgs) (map[string]any, error) { return s.GetAbsence(c, a.ID) })},
		{Info: mcp.ToolInfo{Name: "absence_decide", Title: "Решение по отсутствию", Description: "Согласовать или отклонить заявку (руководитель или его заместитель).",
			InputSchema: schema(map[string]any{"id": str("ID заявки"), "decision": str("Решение", "approved", "rejected"), "comment": str("Комментарий")}, "id", "decision")},
			Roles: approvers,
			Handler: simple(func(c identity.Claims, a idArgs) (map[string]any, error) {
				return s.DecideAbsence(c, a.ID, a.Decision, a.Comment)
			})},
		{Info: mcp.ToolInfo{Name: "access_catalog", Title: "Каталог доступов", Description: "Системы, к которым можно запросить доступ, порядок согласования и текущий статус.",
			Annotations: map[string]any{"readOnlyHint": true}},
			Handler: simple(func(c identity.Claims, _ idArgs) (map[string]any, error) { return s.Catalog(c) })},
		{Info: mcp.ToolInfo{Name: "access_request", Title: "Заявка на доступ",
			Description: "Запросить доступ к системе из каталога. С dry_run=true только проверяет заявку.",
			InputSchema: schema(map[string]any{"system": str("ID системы из каталога"),
				"project":       str("Проект (для систем с проектами, например репозиторий Stash)"),
				"level":         str("Уровень прав: read — чтение, edit — редактирование, admin — администрирование", "read", "edit", "admin"),
				"justification": str("Для какой задачи нужен доступ"), "dry_run": dryRun}, "system")},
			Handler: func(_ context.Context, c identity.Claims, raw json.RawMessage) (map[string]any, error) {
				in, err := decode[AccessInput](raw)
				if err != nil {
					return nil, err
				}
				return s.RequestAccess(c, in)
			}},
		{Info: mcp.ToolInfo{Name: "access_list", Title: "Мои заявки на доступ", Description: "Список заявок на доступ.",
			InputSchema: schema(map[string]any{"scope": scope}), Annotations: map[string]any{"readOnlyHint": true}},
			Handler: simple(func(c identity.Claims, a idArgs) (map[string]any, error) { return s.ListAccess(c, a.Scope) })},
		{Info: mcp.ToolInfo{Name: "access_decide", Title: "Решение по доступу", Description: "Выдать или отклонить доступ (согласующий: руководитель, владелец проекта или DevOps).",
			InputSchema: schema(map[string]any{"id": str("ID заявки"), "decision": str("Решение", "granted", "rejected"), "comment": str("Комментарий")}, "id", "decision")},
			Roles: approvers,
			Handler: simple(func(c identity.Claims, a idArgs) (map[string]any, error) {
				return s.DecideAccess(c, a.ID, a.Decision, a.Comment)
			})},
		{Info: mcp.ToolInfo{Name: "delegation_set", Title: "Заместитель по согласованию",
			Description: "Руководитель передаёт сотруднику своей команды право согласовывать свои заявки (enabled=false — забрать).",
			InputSchema: schema(map[string]any{"user_id": str("ID сотрудника"), "enabled": map[string]any{"type": "boolean"}}, "user_id", "enabled")},
			Roles: leads,
			Handler: func(_ context.Context, c identity.Claims, raw json.RawMessage) (map[string]any, error) {
				in, err := decode[struct {
					UserID  string `json:"user_id"`
					Enabled bool   `json:"enabled"`
				}](raw)
				if err != nil {
					return nil, err
				}
				return s.SetDeputy(c, in.UserID, in.Enabled)
			}},
		{Info: mcp.ToolInfo{Name: "directory_teams", Title: "Отделы", Description: "Отделы компании и их руководители.",
			Annotations: map[string]any{"readOnlyHint": true}},
			Handler: simple(func(identity.Claims, idArgs) (map[string]any, error) { return map[string]any{"teams": s.Teams()}, nil })},
		{Info: mcp.ToolInfo{Name: "directory_transfer", Title: "Перевод в другой отдел",
			Description: "Руководитель переводит сотрудника своего отдела в другой отдел; начальником становится руководитель нового отдела.",
			InputSchema: schema(map[string]any{"user_id": str("ID сотрудника"), "team": str("ID нового отдела")}, "user_id", "team")},
			Roles: leads,
			Handler: func(_ context.Context, c identity.Claims, raw json.RawMessage) (map[string]any, error) {
				in, err := decode[struct {
					UserID string `json:"user_id"`
					Team   string `json:"team"`
				}](raw)
				if err != nil {
					return nil, err
				}
				return s.Transfer(c, in.UserID, in.Team)
			}},
		{Info: mcp.ToolInfo{Name: "directory_newcomers", Title: "Новые сотрудники", Description: "Зарегистрированные сотрудники без отдела.",
			Annotations: map[string]any{"readOnlyHint": true}},
			Roles:   leads,
			Handler: simple(func(c identity.Claims, _ idArgs) (map[string]any, error) { return s.Newcomers(c) })},
		{Info: mcp.ToolInfo{Name: "directory_assign", Title: "Принять в отдел",
			Description: "Руководитель принимает нового сотрудника в свой отдел и назначает роль (intern, engineer, mentor).",
			InputSchema: schema(map[string]any{"user_id": str("ID сотрудника"), "role": str("Роль", "intern", "engineer", "mentor"), "mentor_id": str("Наставник стажёра")}, "user_id", "role")},
			Roles: leads,
			Handler: func(_ context.Context, c identity.Claims, raw json.RawMessage) (map[string]any, error) {
				in, err := decode[struct {
					UserID   string `json:"user_id"`
					Role     string `json:"role"`
					MentorID string `json:"mentor_id"`
				}](raw)
				if err != nil {
					return nil, err
				}
				return s.Assign(c, in.UserID, in.Role, in.MentorID)
			}},
		{Info: mcp.ToolInfo{Name: "delegation_list", Title: "Заместители", Description: "Мои заместители по согласованию и руководители, за которых я согласую.",
			Annotations: map[string]any{"readOnlyHint": true}},
			Handler: simple(func(c identity.Claims, _ idArgs) (map[string]any, error) { return s.Deputies(c) })},
	}
	for _, t := range tools {
		srv.Register(t)
	}
	return srv
}

// Handler — HTTP API имитации: MCP, mock-SSO, health и переключатель сбоя.
func (s *Service) Handler(signer *identity.Signer, demoPassword, adminToken string) http.Handler {
	mux := http.NewServeMux()
	// после перевода в другой отдел права определяет справочник, а не старый токен
	mcpSrv := s.MCPServer(func(tok string) (identity.Claims, error) {
		c, err := signer.Verify(tok)
		if err != nil {
			return c, err
		}
		return s.refresh(c), nil
	})
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Outage() {
			http.Error(w, "service unavailable (simulated outage)", http.StatusServiceUnavailable)
			return
		}
		mcpSrv.ServeHTTP(w, r)
	}))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "outage": s.Outage()})
	})
	// Mock-SSO: в Авито авторизация уже есть, здесь — её имитация.
	mux.HandleFunc("POST /sso/token", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Login, Password string }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request"})
			return
		}
		u, ok := s.CheckPassword(in.Login, in.Password, demoPassword)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_credentials"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"token": signer.Issue(u, 12*time.Hour), "user": u})
	})
	// Регистрация сотрудника: ФИО, почта, пароль, отдел (руководитель) и роль.
	mux.HandleFunc("POST /sso/register", func(w http.ResponseWriter, r *http.Request) {
		var in RegisterInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request", "message": "Некорректный запрос"})
			return
		}
		u, err := s.Register(in)
		var re *RegisterError
		switch {
		case errors.As(err, &re):
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid", "message": re.Msg})
		case err != nil:
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "unavailable"})
		default:
			writeJSON(w, http.StatusCreated, map[string]any{"token": signer.Issue(u, 12*time.Hour), "user": u})
		}
	})
	mux.HandleFunc("GET /sso/teams", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"teams": s.Teams()})
	})
	mux.HandleFunc("GET /sso/users", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"users": s.Users()})
	})
	mux.HandleFunc("POST /admin/outage", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Admin-Token") != adminToken || adminToken == "" {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
			return
		}
		var in struct{ Enabled bool }
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in)
		s.SetOutage(in.Enabled)
		writeJSON(w, http.StatusOK, map[string]any{"outage": in.Enabled})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
