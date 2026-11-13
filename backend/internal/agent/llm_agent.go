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
	"aibuddy/internal/onboarding"
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
	{Type: "function", Function: llm.FunctionSpec{Name: "goals_get",
		Description: "Цели стажировки пользователя: формулировка, критерий успеха, срок, статус и прогресс.",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}}}},
	{Type: "function", Function: llm.FunctionSpec{Name: "plan_step_done",
		Description: "Отметить шаг плана адаптации выполненным. Вызывай, только если стажёр сам сказал, что сделал шаг. Шаги с тестом закрываются сдачей теста.",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{
			"step": map[string]any{"type": "string", "description": "id шага из plan_get или его название"}}, "required": []string{"step"}}}},
	{Type: "function", Function: llm.FunctionSpec{Name: "quiz_list",
		Description: "Тесты на проверку знаний: название, инструкции, которые проверяет тест, сдан ли он пользователем.",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}}}},
	{Type: "function", Function: llm.FunctionSpec{Name: "quiz_offer",
		Description: "Показать под ответом кнопку «Пройти тест». Вызывай, когда стажёр хочет проверить знания или закончил тему, по которой есть тест.",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{
			"quiz_id": map[string]any{"type": "string", "description": "id теста из quiz_list"}}, "required": []string{"quiz_id"}}}},
	{Type: "function", Function: llm.FunctionSpec{Name: "ask_mentor",
		Description: "Подготовить вопрос наставнику, если в базе знаний нет ответа. Стажёр подтвердит отправку.",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{
			"question": map[string]any{"type": "string", "description": "Вопрос от лица стажёра"}}, "required": []string{"question"}}}},
}

// teamTool — только для наставника, руководителя и HR BP: стажёры, которых пользователь видит по правам.
var teamTool = llm.Tool{Type: "function", Function: llm.FunctionSpec{Name: "team_interns",
	Description: "Стажёры, за которых отвечает пользователь (наставник, руководитель, HR BP): день стажировки, прогресс плана, просрочки, следующий шаг, вопросы без ответа, цели, программа, тесты.",
	Parameters:  map[string]any{"type": "object", "properties": map[string]any{}}}}

// writeTools — инструменты с побочным эффектом: агент выполняет их только в
// режиме dry_run, а реальная отправка — после подтверждения стажёром.
var writeTools = map[string]bool{"absence_create": true, "access_request": true}

func (a *Agent) contextBlock(ctx context.Context, u domain.User) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Пользователь: %s, роль: %s, команда: %s.", u.Name, roleNames[u.Role], u.TeamName)
	if u.Role != domain.RoleIntern {
		b.WriteString(" Пользователь не стажёр: он ведёт стажёров или работает с ними, о своих стажёрах спрашивай инструмент team_interns.")
	}
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
	if req.User.Role != domain.RoleIntern && a.d.Team != nil {
		tools = append(tools, teamTool)
	}
	if mcpTools, err := a.d.Corp.ListTools(ctx, req.Token); err == nil {
		for _, t := range mcpTools {
			if strings.HasSuffix(t.Name, "_decide") || strings.HasPrefix(t.Name, "admin_") || t.Name == "delegation_set" || t.Name == "directory_transfer" || t.Name == "directory_assign" { // решения и права — только в интерфейсе руководителя
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
	case "goals_get":
		if a.d.Goals == nil {
			return map[string]any{"goals": []any{}}
		}
		goals, err := a.d.Goals(ctx, req.User)
		if err != nil {
			return map[string]any{"error": "goals_unavailable"}
		}
		return map[string]any{"goals": goals}
	case "team_interns":
		if a.d.Team == nil || req.User.Role == domain.RoleIntern {
			return map[string]any{"error": "not_allowed"}
		}
		list, err := a.d.Team(ctx, req.User)
		if err != nil {
			return map[string]any{"error": "team_unavailable"}
		}
		return map[string]any{"interns": list}
	case "plan_step_done":
		return a.llmStepDone(ctx, req, args)
	case "quiz_list":
		if a.d.Quizzes == nil {
			return map[string]any{"quizzes": []any{}}
		}
		qs, err := a.d.Quizzes(ctx, req.User)
		if err != nil {
			return map[string]any{"error": "quizzes_unavailable"}
		}
		return map[string]any{"quizzes": qs}
	case "quiz_offer":
		id, _ := args["quiz_id"].(string)
		if a.d.Quizzes != nil {
			if qs, err := a.d.Quizzes(ctx, req.User); err == nil {
				for _, q := range qs {
					if q.ID == id {
						reply.QuizID = id
						return map[string]any{"status": "button_shown", "title": q.Title}
					}
				}
			}
		}
		return map[string]any{"error": "not_found"}
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

func (a *Agent) llmStepDone(ctx context.Context, req Request, args map[string]any) map[string]any {
	if a.d.Plan == nil || a.d.SetStep == nil || req.User.Role != domain.RoleIntern {
		return map[string]any{"error": "not_allowed"}
	}
	ref, _ := args["step"].(string)
	p, err := a.d.Plan(ctx, req.User)
	if err != nil {
		return map[string]any{"error": "plan_unavailable"}
	}
	var s *onboarding.StepView
	for _, per := range p.Periods {
		for i := range per.Steps {
			if per.Steps[i].ID == ref {
				s = &per.Steps[i]
			}
		}
	}
	if s == nil {
		s = matchStep(p, ref)
	}
	switch {
	case s == nil:
		return map[string]any{"error": "step_not_found"}
	case s.Status == domain.StepDone:
		return map[string]any{"status": "already_done", "title": s.Title}
	case s.Owner != "intern":
		return map[string]any{"error": "mentor_step", "message": "Этот шаг отмечает наставник"}
	case s.Quiz != "":
		return map[string]any{"error": "quiz_step", "message": "Шаг закрывается сдачей теста", "quiz_id": s.Quiz}
	}
	np, err := a.d.SetStep(ctx, req.User, s.ID, domain.StepDone)
	if err != nil {
		return map[string]any{"error": "failed", "message": err.Error()}
	}
	out := map[string]any{"status": "done", "title": s.Title, "percent": np.Percent, "done": np.Done, "total": np.Total}
	if np.Next != nil {
		out["next"] = np.Next.Title
	}
	return out
}

var roleNames = map[domain.Role]string{domain.RoleIntern: "стажёр", domain.RoleMentor: "наставник", domain.RoleLead: "руководитель",
	domain.RoleHRBP: "HR BP", domain.RoleEngineer: "сотрудник", domain.RoleAdmin: "администратор сервиса", domain.RoleNewcomer: "новый сотрудник"}
