package api

import (
	"strings"
	"testing"

	"aibuddy/internal/app"
	"aibuddy/internal/domain"
)

func TestDeleteRequestsAndQuestions(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna, maria, oleg := e.login("anna"), e.login("maria"), e.login("oleg")
	_, abs := anna.ask("хочу отгул 16 ноября")
	anna.do("POST", "/api/actions/"+abs.ID+"/confirm", nil, nil)
	_, q := anna.ask("спроси наставника: где тестовый стенд поиска?")
	if q == nil {
		t.Fatal("question proposal")
	}
	anna.do("POST", "/api/actions/"+q.ID+"/confirm", nil, nil)
	var resp struct {
		Requests  []app.InternRequest
		Questions []app.InternQuestion
		CanDelete bool
	}
	if code := maria.do("GET", "/api/team/u-anna/requests", nil, &resp); code != 200 || !resp.CanDelete || len(resp.Requests) != 1 || len(resp.Questions) != 1 {
		t.Fatalf("lead view: %d %+v", code, resp)
	}
	if oleg.do("GET", "/api/team/u-anna/requests", nil, &resp); resp.CanDelete {
		t.Fatal("mentor may not delete")
	}
	// удалять могут только руководитель стажёра и администратор
	for _, c := range []*client{oleg, e.login("sergey"), anna} {
		if code := c.do("DELETE", "/api/team/u-anna/requests/"+abs.ID, nil, nil); code != 403 {
			t.Fatalf("foreign delete: %d", code)
		}
	}
	if code := maria.do("DELETE", "/api/team/u-anna/requests/"+abs.ID, nil, nil); code != 200 {
		t.Fatalf("lead delete: %d", code)
	}
	if code := maria.do("DELETE", "/api/team/u-anna/requests/"+abs.ID, nil, nil); code != 404 {
		t.Fatalf("delete twice: %d", code)
	}
	var ap struct{ Approvals []app.Approval }
	maria.do("GET", "/api/approvals", nil, &ap)
	if len(ap.Approvals) != 0 {
		t.Fatalf("deleted from request service: %+v", ap)
	}
	var acts struct{ Actions []domain.Action }
	anna.do("GET", "/api/actions", nil, &acts)
	for _, a := range acts.Actions {
		if a.ID == abs.ID && (a.Status != domain.ActionCancelled || a.Result["deleted_by"] != "Мария Волкова") {
			t.Fatalf("intern action: %+v", a)
		}
	}
	var notes struct{ Notifications []domain.Notification }
	anna.do("GET", "/api/notifications", nil, &notes)
	if !strings.Contains(notes.Notifications[0].Text, "удалил(а) заявку") {
		t.Fatalf("notify: %+v", notes.Notifications[0])
	}
	// вопрос наставнику исчезает у наставника и стажёра, ответить на него нельзя
	maria.do("GET", "/api/team/u-anna/requests", nil, &resp)
	qid := resp.Questions[0].ID
	if code := oleg.do("DELETE", "/api/team/u-anna/questions/"+qid, nil, nil); code != 403 {
		t.Fatal("mentor deletes question")
	}
	if code := maria.do("DELETE", "/api/team/u-anna/questions/"+qid, nil, nil); code != 200 {
		t.Fatalf("delete question: %d", code)
	}
	var inbox struct{ Questions []domain.Question }
	oleg.do("GET", "/api/inbox", nil, &inbox)
	anna.do("GET", "/api/questions", nil, &inbox)
	if len(inbox.Questions) != 0 {
		t.Fatalf("question still visible: %+v", inbox)
	}
	if code := oleg.do("POST", "/api/inbox/"+qid+"/answer", map[string]any{"answer": "search-staging"}, nil); code != 404 {
		t.Fatalf("answer deleted: %d", code)
	}
	if maria.do("GET", "/api/team/u-anna/requests", nil, &resp); len(resp.Questions) != 0 || len(resp.Requests) != 0 {
		t.Fatalf("after delete: %+v", resp)
	}
	// администратор удаляет заявку стажёра любого отдела
	ivan := e.login("ivan")
	_, ia := ivan.ask("хочу отгул 17 ноября")
	ivan.do("POST", "/api/actions/"+ia.ID+"/confirm", nil, nil)
	if code := e.login("admin").do("DELETE", "/api/team/u-ivan/requests/"+ia.ID, nil, nil); code != 200 {
		t.Fatalf("admin delete: %d", code)
	}
	if code := maria.do("DELETE", "/api/team/u-ivan/requests/"+ia.ID, nil, nil); code != 403 {
		t.Fatal("lead of other team")
	}
}
