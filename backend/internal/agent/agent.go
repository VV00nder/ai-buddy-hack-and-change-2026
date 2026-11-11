// Package agent — логика AI-buddy: отвечает на вопросы с учётом контекста
// стажёра и готовит действия через корпоративные сервисы (MCP).
//
// Два режима с одинаковым набором инструментов и одинаковыми правами:
//   - llm: OpenAI-совместимая модель с вызовом инструментов;
//   - offline: детерминированный маршрутизатор намерений — работает без ключа
//     и используется как резерв, если модель недоступна.
package agent

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"aibuddy/internal/domain"
	"aibuddy/internal/knowledge"
	"aibuddy/internal/llm"
	"aibuddy/internal/mcp"
	"aibuddy/internal/onboarding"
)

//go:embed prompts/system.md
var systemPrompt string

// SystemPrompt возвращает системный промпт на дату.
func SystemPrompt(today time.Time) string {
	return strings.ReplaceAll(systemPrompt, "{{today}}", today.Format("2006-01-02")+" ("+weekdayRu[today.Weekday()]+")")
}

var weekdayRu = [...]string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}

// Corp — доступ к корпоративным сервисам (реализуется MCP-клиентом).
type Corp interface {
	Call(ctx context.Context, token, name string, args map[string]any) (map[string]any, error)
	ListTools(ctx context.Context, token string) ([]mcp.ToolInfo, error)
}

// Deps — зависимости агента.
type Deps struct {
	KB   *knowledge.Base
	Corp Corp
	Plan func(ctx context.Context, u domain.User) (onboarding.Plan, error)
	LLM  llm.Client // nil — офлайн-режим
	// Goals — цели стажировки пользователя (необязательно).
	Goals func(ctx context.Context, u domain.User) ([]domain.Goal, error)
	Now   func() time.Time
	// MaxSteps — ограничение шагов вызова инструментов в LLM-режиме.
	MaxSteps int
}

// Request — входящее сообщение.
type Request struct {
	User     domain.User
	Token    string
	Text     string
	History  []domain.Message
	Progress func(domain.TraceItem)
}

// Proposal — черновик действия, которое стажёр подтверждает кнопкой.
type Proposal struct {
	Kind     domain.ActionKind
	Tool     string
	Title    string
	Args     map[string]any
	Preview  map[string]any
	Warnings []string
}

// Reply — ответ агента.
type Reply struct {
	Text        string
	Sources     []domain.Source
	Proposal    *Proposal
	Suggestions []string
	Mode        string
	Context     map[string]string
	// Escalated — в базе знаний не нашлось ответа (метрика «вопрос ушёл наставнику»).
	Escalated bool
	// Answered — ответ найден в базе знаний / сервисах.
	Answered bool
}

// Agent — исполнитель.
type Agent struct{ d Deps }

// New создаёт агента.
func New(d Deps) *Agent {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.MaxSteps == 0 {
		d.MaxSteps = 6
	}
	return &Agent{d: d}
}

// Mode — текущий режим работы.
func (a *Agent) Mode() string {
	if a.d.LLM != nil {
		return "llm"
	}
	return "offline"
}

// Handle обрабатывает сообщение стажёра.
func (a *Agent) Handle(ctx context.Context, req Request) Reply {
	if req.Progress == nil {
		req.Progress = func(domain.TraceItem) {}
	}
	if a.d.LLM != nil {
		r, err := a.handleLLM(ctx, req)
		if err == nil {
			r.Mode = "llm"
			return r
		}
		req.Progress(domain.TraceItem{Kind: "fallback", Label: "Модель недоступна — отвечаю в упрощённом режиме", At: a.d.Now()})
	}
	r := a.handleOffline(ctx, req)
	r.Mode = "offline"
	return r
}

func (a *Agent) step(req Request, kind, label string) {
	req.Progress(domain.TraceItem{Kind: kind, Label: label, At: a.d.Now()})
}

// toolLabels — понятные пользователю подписи шагов (статус ожидания).
var toolLabels = map[string]string{
	"kb_search":        "Ищу в базе знаний команды",
	"plan_get":         "Смотрю твой план адаптации",
	"goals_get":        "Смотрю цели стажировки",
	"ask_mentor":       "Готовлю вопрос наставнику",
	"directory_whoami": "Смотрю профиль в справочнике",
	"directory_team":   "Смотрю состав команды",
	"absence_policy":   "Читаю правила отсутствий",
	"absence_balance":  "Проверяю баланс отгулов",
	"absence_create":   "Проверяю заявку в системе отсутствий",
	"absence_list":     "Смотрю статусы заявок на отсутствие",
	"absence_get":      "Смотрю статус заявки",
	"access_catalog":   "Смотрю каталог доступов",
	"access_request":   "Проверяю заявку в системе доступов",
	"access_list":      "Смотрю статусы заявок на доступ",
}

func toolLabel(name string) string {
	if l, ok := toolLabels[name]; ok {
		return l
	}
	return "Обращаюсь к сервису " + name
}

func sourcesOf(hits []knowledge.Hit) []domain.Source {
	var out []domain.Source
	seen := map[string]bool{}
	for _, h := range hits {
		key := h.DocID + "|" + h.Section
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, domain.Source{DocID: h.DocID, Title: h.Title, Section: h.Section})
	}
	return out
}

func absenceTitle(args map[string]any) string {
	from, _ := args["date_from"].(string)
	to, _ := args["date_to"].(string)
	kind := "Отгул"
	if args["type"] == "vacation" {
		kind = "Отпуск"
	}
	if to == "" || to == from {
		return fmt.Sprintf("%s на %s", kind, ruDate(from))
	}
	return fmt.Sprintf("%s с %s по %s", kind, ruDate(from), ruDate(to))
}

func ruDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("02.01") + " (" + shortWD[t.Weekday()] + ")"
}

var shortWD = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}
