package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewRequiresConfig(t *testing.T) {
	if _, err := New(Config{}); err != ErrNotConfigured {
		t.Fatal(err)
	}
	c, err := New(Config{BaseURL: "http://x", APIKey: "k", Model: "m"})
	if err != nil || c.Model() != "m" || c.cfg.Timeout == 0 {
		t.Fatal("defaults")
	}
}

func TestChat(t *testing.T) {
	var got map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(404)
			return
		}
		got = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"kb_search","arguments":"{\"query\":\"отгул\"}"}}]}}]}`))
	}))
	defer ts.Close()
	c, _ := New(Config{BaseURL: ts.URL + "/v1/", APIKey: "k", Model: "gpt-x", Timeout: time.Second})
	msg, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "привет"}},
		[]Tool{{Type: "function", Function: FunctionSpec{Name: "kb_search", Parameters: map[string]any{"type": "object"}}}})
	if err != nil || len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "kb_search" {
		t.Fatalf("chat: %+v %v", msg, err)
	}
	if got["model"] != "gpt-x" || got["tool_choice"] != "auto" {
		t.Fatalf("request: %v", got)
	}
	// без инструментов tool_choice не передаётся
	_, _ = c.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil)
	if _, ok := got["tools"]; ok {
		t.Fatal("tools must be omitted")
	}
}

func TestChatErrors(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"http 500": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(500)
			_, _ = w.Write([]byte(strings.Repeat("e", 400)))
		},
		"llm decode":    func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("nope")) },
		"empty choices": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"choices":[]}`)) },
	}
	for want, h := range cases {
		ts := httptest.NewServer(h)
		c, _ := New(Config{BaseURL: ts.URL, APIKey: "k", Model: "m", Timeout: time.Second})
		_, err := c.Chat(context.Background(), nil, nil)
		ts.Close()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: %v", want, err)
		}
	}
	c, _ := New(Config{BaseURL: "http://127.0.0.1:1", APIKey: "k", Model: "m", Timeout: 200 * time.Millisecond})
	if _, err := c.Chat(context.Background(), nil, nil); err == nil {
		t.Fatal("unreachable")
	}
	c, _ = New(Config{BaseURL: "::bad", APIKey: "k", Model: "m"})
	if _, err := c.Chat(context.Background(), nil, nil); err == nil {
		t.Fatal("bad url")
	}
	c, _ = New(Config{BaseURL: "http://x", APIKey: "k", Model: "m"})
	if _, err := c.Chat(context.Background(), nil, []Tool{{Function: FunctionSpec{Parameters: map[string]any{"f": func() {}}}}}); err == nil {
		t.Fatal("marshal")
	}
}
