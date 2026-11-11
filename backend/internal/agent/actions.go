package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"aibuddy/internal/domain"
	"aibuddy/internal/mcp"
)

// prepareAbsence проверяет заявку в сервисе (dry_run) и возвращает черновик
// или бизнес-ошибку, которую нужно объяснить пользователю.
func (a *Agent) prepareAbsence(ctx context.Context, req Request, args map[string]any) (*Proposal, *mcp.CallError, error) {
	args = cloneArgs(args)
	if args["type"] == nil || args["type"] == "" {
		args["type"] = "day_off"
	}
	if to, _ := args["date_to"].(string); to == "" {
		args["date_to"] = args["date_from"]
	}
	args["dry_run"] = true
	a.step(req, "tool", toolLabel("absence_create"))
	prev, err := a.d.Corp.Call(ctx, req.Token, "absence_create", args)
	var ce *mcp.CallError
	if errors.As(err, &ce) {
		return nil, ce, nil
	}
	if err != nil {
		return nil, nil, err
	}
	delete(args, "dry_run")
	p := &Proposal{Kind: domain.ActionAbsence, Tool: "absence_create", Title: absenceTitle(args), Args: args, Preview: prev}
	return p, nil, nil
}

// prepareAccess — то же для доступа.
func (a *Agent) prepareAccess(ctx context.Context, req Request, args map[string]any) (*Proposal, *mcp.CallError, error) {
	args = cloneArgs(args)
	args["dry_run"] = true
	a.step(req, "tool", toolLabel("access_request"))
	prev, err := a.d.Corp.Call(ctx, req.Token, "access_request", args)
	var ce *mcp.CallError
	if errors.As(err, &ce) {
		return nil, ce, nil
	}
	if err != nil {
		return nil, nil, err
	}
	delete(args, "dry_run")
	name, _ := prev["system_name"].(string)
	if pr, _ := prev["project"].(string); pr != "" {
		name += " — " + pr
	}
	p := &Proposal{Kind: domain.ActionAccess, Tool: "access_request", Title: name, Args: args, Preview: prev}
	return p, nil, nil
}

// prepareMentorQuestion — черновик вопроса наставнику (сервис AI-buddy).
func prepareMentorQuestion(question string) *Proposal {
	q := strings.TrimSpace(question)
	return &Proposal{Kind: domain.ActionMentorQuestion, Tool: "ask_mentor", Title: "Вопрос наставнику",
		Args: map[string]any{"question": q}, Preview: map[string]any{"question": q}}
}

func cloneArgs(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

// explainCallError превращает ошибку сервиса в понятный ответ.
func explainCallError(ce *mcp.CallError) string {
	if ce.Code == "forbidden" {
		return "У тебя нет прав на эту операцию — я не могу её выполнить. Если доступ нужен для задачи, обсуди это с руководителем."
	}
	return ce.Message
}

// unavailableText — сообщение при сбое корпоративного сервиса.
func unavailableText(service string) string {
	return fmt.Sprintf("Сервис %s сейчас не отвечает, поэтому я не смог выполнить запрос. Попробуй ещё раз через пару минут — я ничего не отправил и данные не потеряны.", service)
}
