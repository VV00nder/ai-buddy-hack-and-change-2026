// Package mcp — минимальная реализация Model Context Protocol (JSON-RPC 2.0
// поверх HTTP, транспорт Streamable HTTP с JSON-ответами).
//
// Корпоративные сервисы (отсутствия, доступы, справочник) публикуются как
// MCP-серверы, а AI-buddy подключается к ним MCP-клиентом. Так же их сможет
// подключить внутренняя платформа ИИ-агентов Авито без переделки.
package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ProtocolVersion — версия протокола MCP, которую поддерживает реализация.
const ProtocolVersion = "2025-06-18"

// JSON-RPC коды ошибок.
const (
	CodeParse          = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeUnauthorized   = -32001
)

// Request — запрос JSON-RPC.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response — ответ JSON-RPC.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError — ошибка JSON-RPC.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("mcp rpc error %d: %s", e.Code, e.Message) }

// ToolInfo — описание инструмента в tools/list.
type ToolInfo struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

// Content — элемент содержимого результата.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// CallResult — результат tools/call.
type CallResult struct {
	Content           []Content      `json:"content"`
	StructuredContent map[string]any `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
}

// ToolError — бизнес-ошибка инструмента (возвращается как isError=true,
// чтобы агент мог объяснить её пользователю).
type ToolError struct {
	Code    string
	Message string
}

func (e *ToolError) Error() string { return e.Message }

// NewToolError создаёт бизнес-ошибку.
func NewToolError(code, msg string) *ToolError { return &ToolError{Code: code, Message: msg} }

// ErrForbidden — у пользователя нет прав на операцию.
var ErrForbidden = errors.New("forbidden")
