// Package llm — клиент OpenAI-совместимого Chat Completions API с поддержкой
// вызова инструментов (tool calling). Подходит для OpenAI, OpenRouter,
// YandexGPT/GigaChat через совместимые шлюзы, vLLM и Ollama.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Message — сообщение диалога.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall — вызов инструмента моделью.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Tool — описание инструмента для модели.
type Tool struct {
	Type     string       `json:"type"`
	Function FunctionSpec `json:"function"`
}

// FunctionSpec — сигнатура функции.
type FunctionSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Client — клиент модели.
type Client interface {
	Chat(ctx context.Context, msgs []Message, tools []Tool) (Message, error)
	Model() string
}

// Config — параметры подключения.
type Config struct {
	BaseURL     string
	APIKey      string
	Model       string
	Temperature float64
	Timeout     time.Duration
}

// HTTPClient — реализация поверх HTTP.
type HTTPClient struct {
	cfg  Config
	http *http.Client
}

// ErrNotConfigured — ключ или адрес не заданы.
var ErrNotConfigured = errors.New("llm: not configured")

// New создаёт клиента; без ключа возвращает ErrNotConfigured.
func New(cfg Config) (*HTTPClient, error) {
	if cfg.APIKey == "" || cfg.BaseURL == "" || cfg.Model == "" {
		return nil, ErrNotConfigured
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 45 * time.Second
	}
	return &HTTPClient{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}, nil
}

// Model возвращает имя модели.
func (c *HTTPClient) Model() string { return c.cfg.Model }

// Chat выполняет один шаг диалога.
func (c *HTTPClient) Chat(ctx context.Context, msgs []Message, tools []Tool) (Message, error) {
	body := map[string]any{"model": c.cfg.Model, "messages": msgs, "temperature": c.cfg.Temperature}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return Message{}, err
	}
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return Message{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return Message{}, fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return Message{}, fmt.Errorf("llm http %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	var out struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return Message{}, fmt.Errorf("llm decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return Message{}, errors.New("llm: empty choices")
	}
	return out.Choices[0].Message, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
