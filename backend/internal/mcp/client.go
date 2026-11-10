package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// Client — MCP-клиент для одного сервера.
type Client struct {
	URL  string
	HTTP *http.Client
	seq  atomic.Int64
}

// NewClient создаёт клиента с таймаутом.
func NewClient(url string, timeout time.Duration) *Client {
	return &Client{URL: url, HTTP: &http.Client{Timeout: timeout}}
}

// CallError — инструмент вернул бизнес-ошибку (isError=true).
type CallError struct {
	Code    string
	Message string
}

func (e *CallError) Error() string { return e.Message }

func (c *Client) rpc(ctx context.Context, token, method string, params any, out any) error {
	id := c.seq.Add(1)
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", ProtocolVersion)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("mcp: %s unavailable: %w", c.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return &UnavailableError{Status: resp.StatusCode}
	}
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  *RPCError       `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("mcp: bad response (http %d): %w", resp.StatusCode, err)
	}
	if r.Error != nil {
		return r.Error
	}
	return json.Unmarshal(r.Result, out)
}

// UnavailableError — сервис ответил 5xx.
type UnavailableError struct{ Status int }

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("mcp: service unavailable (http %d)", e.Status)
}

// Initialize выполняет рукопожатие.
func (c *Client) Initialize(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.rpc(ctx, "", "initialize", map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "ai-buddy", "version": "1.0"},
	}, &out)
	return out, err
}

// ListTools возвращает инструменты, доступные пользователю с токеном.
func (c *Client) ListTools(ctx context.Context, token string) ([]ToolInfo, error) {
	var out struct {
		Tools []ToolInfo `json:"tools"`
	}
	err := c.rpc(ctx, token, "tools/list", map[string]any{}, &out)
	return out.Tools, err
}

// Call вызывает инструмент от имени пользователя.
func (c *Client) Call(ctx context.Context, token, name string, args map[string]any) (map[string]any, error) {
	if args == nil {
		args = map[string]any{}
	}
	var res CallResult
	if err := c.rpc(ctx, token, "tools/call", map[string]any{"name": name, "arguments": args}, &res); err != nil {
		return nil, err
	}
	if res.IsError {
		code, _ := res.StructuredContent["error"].(string)
		msg, _ := res.StructuredContent["message"].(string)
		if msg == "" && len(res.Content) > 0 {
			msg = res.Content[0].Text
		}
		return res.StructuredContent, &CallError{Code: code, Message: msg}
	}
	if res.StructuredContent == nil && len(res.Content) > 0 {
		var m map[string]any
		if json.Unmarshal([]byte(res.Content[0].Text), &m) == nil {
			return m, nil
		}
	}
	return res.StructuredContent, nil
}
