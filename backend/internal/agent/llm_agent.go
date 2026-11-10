package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aibuddy/internal/domain"
	"aibuddy/internal/knowledge"
	"aibuddy/internal/llm"
	"aibuddy/internal/mcp"
)

// localTools — инструменты самого AI-buddy (поверх MCP-инструментов сервисов).
var localTools = []llm.Tool{
	{Type: "function", Function: llm.FunctionSpec{Name: "kb_search",
		Description: "Поиск по базе знаний команды (процессы, инструменты, договорённости). Возвращает разделы инструкций.",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Поисковый запрос"}}, "required": []string{"query"}}}},
	{Type: "function", Function: llm.FunctionSpec{Name: "plan_get",
		Description: "План адаптации стажёра: день стажировки, прогресс, следующий шаг, шаги текущего этапа.",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}}}},
	{Type: "function", Function: llm.FunctionSpec{Name: "ask_mentor",
		Description: "Подготовить вопрос наставнику, если в базе знаний нет ответа. Стажёр подтвердит отправку.",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{
			"question": map[string]any{"type": "string", "description": "Вопрос от лица стажёра"}}, "required": []string{"question"}}}},
}

// writeTools — инструменты с побочным эффектом: агент выполняет их только в
// режиме dry_run, а реальная отправка — после подтверждения стажёром.
var writeTools = map[string]bool{"absence_create": true, "access_request": true}

func (a *Agent) contextBlock(ctx context.Context, u domain.User) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Пользователь: %s, роль: %s, команда: %s.", u.Name, u.Role, u.TeamName)
	if a.d.Plan != nil && u.Role == domain.RoleIntern {
		if p, err := a.d.Plan(ctx, u); err == nil {
			fmt.Fprintf(&b, " День стажировки: %d, прогресс плана: %d%%.", p.Day, p.Percent)
			if p.Next != nil {
				fmt.Fprintf(&b, " Следующий шаг плана: «%s».", p.Next.Title)
			}
		}
	}
	return b.String()
}

func (a *Agent) handleLLM(ctx context.Context, req Request) (Reply, error) {
	a.step(req, "thinking", "Думаю над ответом")
	tools := append([]llm.Tool{}, localTools...)
	if mcpTools, err := a.d.Corp.ListTools(ctx, req.Token); err == nil {
		for _, t := range mcpTools {
			if strings.HasSuffix(t.Name, "_decide") { // решения принимаются только в интерфейсе руководителя
				continue
			}
			tools = append(tools, llm.Tool{Type: "function", Function: llm.FunctionSpec{Name: t.Name, Description: t.Description, Parameters: t.InputSchema}})
		}
	}
	msgs := []llm.Message{
		{Role: "system", Content: SystemPrompt(a.d.Now()) + "\n\nКонтекст: " + a.contextBlock(ctx, req.User)},
	}
	hist := req.History
	if len(hist) > 10 {
		hist = hist[len(hist)-10:]
	}
	for _, m := range hist {
		if m.Status != domain.MsgDone || m.Text == "" {
			continue
		}
		role := "user"
		if m.Author == "assistant" {
			role = "assistant"
		}
		msgs = append(msgs, llm.Message{Role: role, Content: m.Text})
	}
	msgs = append(msgs, llm.Message{Role: "user", Content: req.Text})

	var reply Reply
	var hits []knowledge.Hit
	for i := 0; i < a.d.MaxSteps; i++ {
		out, err := a.d.LLM.Chat(ctx, msgs, tools)
		if err != nil {
			return Reply{}, err
		}
		if len(out.ToolCalls) == 0 {
			reply.Text = strings.TrimSpace(out.Content)
			if reply.Text == "" {
				return Reply{}, errors.New("llm: empty answer")
			}
			reply.Sources = sourcesOf(hits)
			reply.Answered = reply.Proposal == nil || reply.Proposal.Kind != domain.ActionMentorQuestion
			reply.Escalated = !reply.Answered
			return reply, nil
		}
		msgs = append(msgs, llm.Message{Role: "assistant", Content: out.Content, ToolCalls: out.ToolCalls})
		for _, tc := range out.ToolCalls {
			result := a.runTool(ctx, req, tc, &reply, &hits)
			b, _ := json.Marshal(result)
			msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: string(b)})
		}
	}
	return Reply{}, errors.New("llm: too many tool steps")
}

func (a *Agent) runTool(ctx context.Context, req Request, tc llm.ToolCall, reply *Reply, hits *[]knowledge.Hit) map[string]any {
	name := tc.Function.Name
	a.step(req, "tool", toolLabel(name))
	args := map[string]any{}
	if tc.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return map[string]any{"error": "bad_arguments"}
		}
	}
	switch name {
	case "kb_search":
		q, _ := args["query"].(string)
		h := a.d.KB.Search(q, req.User, 4)
		*hits = append(*hits, h...)
		items := []map[string]any{}
		for _, x := range h {
			items = append(items, map[string]any{"doc": x.Title, "section": x.Section, "text": x.Text, "kind": x.Kind})
		}
		return map[string]any{"results": items, "note": "Инструкции тестовые, не регламент Авито"}
	case "plan_get":
		p, err := a.d.Plan(ctx, req.User)
		if err != nil {
			return map[string]any{"error": "plan_unavailable"}
		}
		out := map[string]any{"day": p.Day, "done": p.Done, "total": p.Total, "percent": p.Percent, "overdue": p.Overdue, "currentPeriod": p.CurrentPeriod}
		if p.Next != nil {
			out["next"] = map[string]any{"title": p.Next.Title, "description": p.Next.Description}
		}
		return out
	case "ask_mentor":
		q, _ := args["question"].(string)
		if q == "" {
			q = req.Text
		}
		reply.Proposal = prepareMentorQuestion(q)
		return map[string]any{"status": "draft_created", "note": "Стажёр увидит карточку и сам отправит вопрос наставнику."}
	}
	if writeTools[name] {
		var p *Proposal
		var ce *mcp.CallError
		var err error
		if name == "absence_create" {
			p, ce, err = a.prepareAbsence(ctx, req, args)
		} else {
			p, ce, err = a.prepareAccess(ctx, req, args)
		}
		switch {
		case err != nil:
			return map[string]any{"error": "service_unavailable"}
		case ce != nil:
			return map[string]any{"error": ce.Code, "message": ce.Message}
		}
		reply.Proposal = p
		return map[string]any{"status": "draft_created", "preview": p.Preview,
			"note": "Заявка ещё НЕ отправлена: стажёр увидит карточку и подтвердит отправку кнопкой."}
	}
	out, err := a.d.Corp.Call(ctx, req.Token, name, args)
	var ce *mcp.CallError
	if errors.As(err, &ce) {
		return map[string]any{"error": ce.Code, "message": ce.Message}
	}
	if err != nil {
		return map[string]any{"error": "service_unavailable"}
	}
	return out
}
