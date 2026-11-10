package corp

import (
	"context"
	"encoding/json"
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

var leads = []string{string(domain.RoleLead)}

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
		{Info: mcp.ToolInfo{Name: "absence_decide", Title: "Решение по отсутствию", Description: "Согласовать или отклонить заявку (руководитель).",
			InputSchema: schema(map[string]any{"id": str("ID заявки"), "decision": str("Решение", "approved", "rejected"), "comment": str("Комментарий")}, "id", "decision")},
			Roles: leads,
			Handler: simple(func(c identity.Claims, a idArgs) (map[string]any, error) {
				return s.DecideAbsence(c, a.ID, a.Decision, a.Comment)
			})},
		{Info: mcp.ToolInfo{Name: "access_catalog", Title: "Каталог доступов", Description: "Системы, к которым можно запросить доступ, порядок согласования и текущий статус.",
			Annotations: map[string]any{"readOnlyHint": true}},
			Handler: simple(func(c identity.Claims, _ idArgs) (map[string]any, error) { return s.Catalog(c) })},
		{Info: mcp.ToolInfo{Name: "access_request", Title: "Заявка на доступ",
			Description: "Запросить доступ к системе из каталога. С dry_run=true только проверяет заявку.",
			InputSchema: schema(map[string]any{"system": str("ID системы из каталога"), "level": str("Уровень доступа"),
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
		{Info: mcp.ToolInfo{Name: "access_decide", Title: "Решение по доступу", Description: "Выдать или отклонить доступ (руководитель).",
			InputSchema: schema(map[string]any{"id": str("ID заявки"), "decision": str("Решение", "granted", "rejected"), "comment": str("Комментарий")}, "id", "decision")},
			Roles: leads,
			Handler: simple(func(c identity.Claims, a idArgs) (map[string]any, error) {
				return s.DecideAccess(c, a.ID, a.Decision, a.Comment)
			})},
	}
	for _, t := range tools {
		srv.Register(t)
	}
	return srv
}

// Handler — HTTP API имитации: MCP, mock-SSO, health и переключатель сбоя.
func (s *Service) Handler(signer *identity.Signer, demoPassword, adminToken string) http.Handler {
	mux := http.NewServeMux()
	mcpSrv := s.MCPServer(signer.Verify)
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
		u, ok := s.UserByLogin(in.Login)
		if !ok || in.Password != demoPassword {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_credentials"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"token": signer.Issue(u, 12*time.Hour), "user": u})
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
