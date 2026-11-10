package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"

	"aibuddy/internal/identity"
)

// Handler выполняет инструмент от имени вызывающего пользователя.
type Handler func(ctx context.Context, caller identity.Claims, args json.RawMessage) (map[string]any, error)

// Tool — инструмент сервера.
type Tool struct {
	Info ToolInfo
	// Roles — роли, которым инструмент виден и доступен. Пусто — всем.
	Roles   []string
	Handler Handler
}

// Verifier проверяет токен вызывающего пользователя.
type Verifier func(token string) (identity.Claims, error)

// Server — MCP-сервер.
type Server struct {
	name, version string
	verify        Verifier
	mu            sync.RWMutex
	tools         map[string]Tool
	order         []string
}

// NewServer создаёт сервер.
func NewServer(name, version string, verify Verifier) *Server {
	return &Server{name: name, version: version, verify: verify, tools: map[string]Tool{}}
}

// Register добавляет инструмент.
func (s *Server) Register(t Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tools[t.Info.Name]; !ok {
		s.order = append(s.order, t.Info.Name)
	}
	if t.Info.InputSchema == nil {
		t.Info.InputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	s.tools[t.Info.Name] = t
}

func allowed(t Tool, role string) bool {
	return len(t.Roles) == 0 || slices.Contains(t.Roles, role)
}

// ServeHTTP обрабатывает POST с JSON-RPC сообщением.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeRPC(w, Response{JSONRPC: "2.0", Error: &RPCError{Code: CodeParse, Message: "read error"}})
		return
	}
	var req Request
	if err := json.Unmarshal(body, &req); err != nil || req.JSONRPC != "2.0" || req.Method == "" {
		writeRPC(w, Response{JSONRPC: "2.0", Error: &RPCError{Code: CodeInvalidRequest, Message: "invalid request"}})
		return
	}
	if len(req.ID) == 0 { // уведомление (notifications/initialized и т.п.)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	resp := Response{JSONRPC: "2.0", ID: req.ID}
	result, rpcErr := s.dispatch(r.Context(), r.Header.Get("Authorization"), req)
	if rpcErr != nil {
		resp.Error = rpcErr
		if rpcErr.Code == CodeUnauthorized {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
	} else {
		resp.Result = result
	}
	writeRPC(w, resp)
}

func writeRPC(w http.ResponseWriter, resp Response) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) dispatch(ctx context.Context, auth string, req Request) (any, *RPCError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
		}, nil
	case "ping":
		return map[string]any{}, nil
	}
	token, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok {
		return nil, &RPCError{Code: CodeUnauthorized, Message: "missing bearer token"}
	}
	caller, err := s.verify(token)
	if err != nil {
		return nil, &RPCError{Code: CodeUnauthorized, Message: "invalid token"}
	}
	switch req.Method {
	case "tools/list":
		s.mu.RLock()
		defer s.mu.RUnlock()
		list := []ToolInfo{}
		for _, name := range s.order {
			if t := s.tools[name]; allowed(t, string(caller.Role)) {
				list = append(list, t.Info)
			}
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			return nil, &RPCError{Code: CodeInvalidParams, Message: "invalid params"}
		}
		s.mu.RLock()
		t, ok := s.tools[p.Name]
		s.mu.RUnlock()
		if !ok {
			return nil, &RPCError{Code: CodeInvalidParams, Message: "unknown tool: " + p.Name}
		}
		if !allowed(t, string(caller.Role)) {
			return toolErrorResult("forbidden", "Недостаточно прав для этой операции"), nil
		}
		if len(p.Arguments) == 0 {
			p.Arguments = json.RawMessage(`{}`)
		}
		out, err := t.Handler(ctx, caller, p.Arguments)
		if err != nil {
			var te *ToolError
			if errors.As(err, &te) {
				return toolErrorResult(te.Code, te.Message), nil
			}
			if errors.Is(err, ErrForbidden) {
				return toolErrorResult("forbidden", "Недостаточно прав для этой операции"), nil
			}
			return toolErrorResult("internal", "Внутренняя ошибка сервиса"), nil
		}
		text, _ := json.Marshal(out)
		return CallResult{Content: []Content{{Type: "text", Text: string(text)}}, StructuredContent: out}, nil
	}
	return nil, &RPCError{Code: CodeMethodNotFound, Message: "method not found: " + req.Method}
}

func toolErrorResult(code, msg string) CallResult {
	out := map[string]any{"error": code, "message": msg}
	text, _ := json.Marshal(out)
	return CallResult{Content: []Content{{Type: "text", Text: string(text)}}, StructuredContent: out, IsError: true}
}
