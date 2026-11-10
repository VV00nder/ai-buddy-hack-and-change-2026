package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aibuddy/internal/identity"
)

func newTestServer(t *testing.T) (*httptest.Server, func(role string) string) {
	t.Helper()
	tokens := map[string]identity.Claims{"intern": {Sub: "u1", Role: "intern"}, "lead": {Sub: "u2", Role: "lead"}}
	s := NewServer("test", "1", func(tok string) (identity.Claims, error) {
		c, ok := tokens[tok]
		if !ok {
			return c, errors.New("bad")
		}
		return c, nil
	})
	s.Register(Tool{Info: ToolInfo{Name: "echo", Description: "echo"}, Handler: func(_ context.Context, c identity.Claims, a json.RawMessage) (map[string]any, error) {
		var m map[string]any
		_ = json.Unmarshal(a, &m)
		m["caller"] = c.Sub
		return m, nil
	}})
	s.Register(Tool{Info: ToolInfo{Name: "lead.only", Description: "x"}, Roles: []string{"lead"}, Handler: func(context.Context, identity.Claims, json.RawMessage) (map[string]any, error) {
		return map[string]any{"ok": true}, nil
	}})
	s.Register(Tool{Info: ToolInfo{Name: "fail.biz", Description: "x"}, Handler: func(context.Context, identity.Claims, json.RawMessage) (map[string]any, error) {
		return nil, NewToolError("too_late", "Слишком поздно")
	}})
	s.Register(Tool{Info: ToolInfo{Name: "fail.forbidden", Description: "x"}, Handler: func(context.Context, identity.Claims, json.RawMessage) (map[string]any, error) {
		return nil, ErrForbidden
	}})
	s.Register(Tool{Info: ToolInfo{Name: "fail.internal", Description: "x"}, Handler: func(context.Context, identity.Claims, json.RawMessage) (map[string]any, error) {
		return nil, errors.New("db down")
	}})
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	return ts, func(r string) string { return r }
}

func TestClientServer(t *testing.T) {
	ts, tok := newTestServer(t)
	c := NewClient(ts.URL, 2*time.Second)
	ctx := context.Background()
	info, err := c.Initialize(ctx)
	if err != nil || info["protocolVersion"] != ProtocolVersion {
		t.Fatalf("init: %v %v", info, err)
	}
	tools, err := c.ListTools(ctx, tok("intern"))
	if err != nil || len(tools) != 4 {
		t.Fatalf("intern tools: %v %v", len(tools), err)
	}
	tools, _ = c.ListTools(ctx, tok("lead"))
	if len(tools) != 5 {
		t.Fatalf("lead tools: %d", len(tools))
	}
	out, err := c.Call(ctx, tok("intern"), "echo", map[string]any{"a": "b"})
	if err != nil || out["a"] != "b" || out["caller"] != "u1" {
		t.Fatalf("echo: %v %v", out, err)
	}
	if _, err := c.Call(ctx, tok("intern"), "echo", nil); err != nil {
		t.Fatal(err)
	}
	_, err = c.Call(ctx, tok("intern"), "lead.only", nil)
	var ce *CallError
	if !errors.As(err, &ce) || ce.Code != "forbidden" {
		t.Fatalf("want forbidden, got %v", err)
	}
	for name, code := range map[string]string{"fail.biz": "too_late", "fail.forbidden": "forbidden", "fail.internal": "internal"} {
		_, err = c.Call(ctx, tok("intern"), name, nil)
		if !errors.As(err, &ce) || ce.Code != code || ce.Error() == "" {
			t.Fatalf("%s: want %s, got %v", name, code, err)
		}
	}
	var re *RPCError
	if _, err = c.Call(ctx, tok("intern"), "nope", nil); !errors.As(err, &re) || re.Code != CodeInvalidParams {
		t.Fatalf("unknown tool: %v", err)
	}
	if _, err = c.ListTools(ctx, "bad"); !errors.As(err, &re) || re.Code != CodeUnauthorized || re.Error() == "" {
		t.Fatalf("bad token: %v", err)
	}
	if _, err = c.ListTools(ctx, ""); !errors.As(err, &re) {
		t.Fatalf("no token: %v", err)
	}
}

func post(t *testing.T, url, auth, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

func TestServerProtocolEdges(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, _ := http.Get(ts.URL)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", resp.StatusCode)
	}
	code, m := post(t, ts.URL, "", "{bad")
	if code != 200 || m["error"] == nil {
		t.Fatalf("parse: %v", m)
	}
	code, _ = post(t, ts.URL, "", `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if code != http.StatusAccepted {
		t.Fatalf("notification: %d", code)
	}
	_, m = post(t, ts.URL, "", `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	if m["error"] != nil {
		t.Fatalf("ping: %v", m)
	}
	_, m = post(t, ts.URL, "intern", `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`)
	if m["error"].(map[string]any)["code"].(float64) != CodeMethodNotFound {
		t.Fatalf("method not found: %v", m)
	}
	_, m = post(t, ts.URL, "intern", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":"x"}`)
	if m["error"].(map[string]any)["code"].(float64) != CodeInvalidParams {
		t.Fatalf("invalid params: %v", m)
	}
	code, _ = post(t, ts.URL, "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if code != http.StatusUnauthorized {
		t.Fatalf("unauthorized: %d", code)
	}
	// повторная регистрация заменяет инструмент
	s := NewServer("x", "1", nil)
	s.Register(Tool{Info: ToolInfo{Name: "a"}})
	s.Register(Tool{Info: ToolInfo{Name: "a"}})
	if len(s.order) != 1 || s.tools["a"].Info.InputSchema == nil {
		t.Fatal("register")
	}
}

func TestClientTransportErrors(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", 200*time.Millisecond)
	if _, err := c.ListTools(context.Background(), "x"); err == nil {
		t.Fatal("want unavailable")
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("oops")) }))
	defer bad.Close()
	if _, err := NewClient(bad.URL, time.Second).ListTools(context.Background(), "x"); err == nil {
		t.Fatal("want decode error")
	}
	// structuredContent отсутствует — берём JSON из text
	txt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"x\":1}"}]}}`))
	}))
	defer txt.Close()
	out, err := NewClient(txt.URL, time.Second).Call(context.Background(), "t", "a", nil)
	if err != nil || out["x"].(float64) != 1 {
		t.Fatalf("text fallback: %v %v", out, err)
	}
	errTxt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"isError":true,"content":[{"type":"text","text":"boom"}]}}`))
	}))
	defer errTxt.Close()
	_, err = NewClient(errTxt.URL, time.Second).Call(context.Background(), "t", "a", nil)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err text: %v", err)
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer down.Close()
	var ue *UnavailableError
	if _, err := NewClient(down.URL, time.Second).ListTools(context.Background(), "x"); !errors.As(err, &ue) || ue.Error() == "" {
		t.Fatalf("want unavailable, got %v", err)
	}
	if _, err := NewClient("::bad", time.Second).ListTools(context.Background(), ""); err == nil {
		t.Fatal("bad url")
	}
	if _, err := NewClient("http://x", time.Second).Call(context.Background(), "", "a", map[string]any{"f": func() {}}); err == nil {
		t.Fatal("marshal error")
	}
}
