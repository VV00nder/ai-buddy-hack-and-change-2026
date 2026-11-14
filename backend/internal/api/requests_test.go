package api

import (
	"testing"

	"aibuddy/internal/app"
	"aibuddy/internal/domain"
)

// Вкладка «Заявки и вопросы»: руководитель видит все заявки и вопросы своих стажёров,
// заместитель с правом «Согласование заявок» — тоже, и может отвечать на вопросы.
func TestRequestsAndQuestionsTab(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna, maria, elena := e.login("anna"), e.login("maria"), e.login("elena")
	_, abs := anna.ask("хочу отгул 16 ноября")
	anna.do("POST", "/api/actions/"+abs.ID+"/confirm", nil, nil)
	_, q := anna.ask("спроси наставника: где тестовый стенд поиска?")
	anna.do("POST", "/api/actions/"+q.ID+"/confirm", nil, nil)

	var rs struct{ Requests []app.InternRequest }
	if code := maria.do("GET", "/api/requests", nil, &rs); code != 200 || len(rs.Requests) != 1 ||
		!rs.Requests[0].AwaitingMe || rs.Requests[0].InternName != "Анна Смирнова" || rs.Requests[0].Approval == nil {
		t.Fatalf("lead requests: %d %+v", code, rs)
	}
	if code := anna.do("GET", "/api/requests", nil, nil); code != 403 {
		t.Fatalf("intern requests: %d", code)
	}
	// до выдачи права HR BP не видит ни заявок, ни вопросов
	if code := elena.do("GET", "/api/inbox", nil, nil); code != 403 {
		t.Fatalf("elena inbox before grant: %d", code)
	}
	if code := maria.do("POST", "/api/rights", map[string]any{"userId": "u-elena", "perm": "approvals", "enabled": true}, nil); code != 200 {
		t.Fatalf("grant: %d", code)
	}
	elena = e.login("elena")
	var me struct{ Perms []string }
	elena.do("GET", "/api/me", nil, &me)
	if len(me.Perms) == 0 || me.Perms[len(me.Perms)-1] != "approvals" {
		t.Fatalf("perms: %+v", me.Perms)
	}
	if elena.do("GET", "/api/requests", nil, &rs); len(rs.Requests) != 1 || !rs.Requests[0].AwaitingMe {
		t.Fatalf("deputy requests: %+v", rs)
	}
	var inbox struct{ Questions []domain.Question }
	if code := elena.do("GET", "/api/inbox", nil, &inbox); code != 200 || len(inbox.Questions) != 1 {
		t.Fatalf("deputy inbox: %d %+v", code, inbox)
	}
	if code := elena.do("POST", "/api/inbox/"+inbox.Questions[0].ID+"/answer", map[string]any{"answer": "Стенд — search-staging"}, nil); code != 200 {
		t.Fatalf("deputy answers: %d", code)
	}
	// чужой отдел — нет
	if code := e.login("dmitry").do("POST", "/api/inbox/"+inbox.Questions[0].ID+"/answer", map[string]any{"answer": "x y"}, nil); code != 403 {
		t.Fatal("other team mentor")
	}
	// после отзыва права вопросы снова недоступны
	maria.do("POST", "/api/rights", map[string]any{"userId": "u-elena", "perm": "approvals", "enabled": false}, nil)
	if code := e.login("elena").do("GET", "/api/inbox", nil, nil); code != 403 {
		t.Fatalf("after revoke: %d", code)
	}
}
