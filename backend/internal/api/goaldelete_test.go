package api

import (
	"strings"
	"testing"

	"aibuddy/internal/domain"
)

func TestDeleteGoalAndLongAnswers(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna, maria, oleg, admin := e.login("anna"), e.login("maria"), e.login("oleg"), e.login("admin")
	add := func(c *client, title string) string {
		var out struct{ Goal domain.Goal }
		if code := c.do("POST", "/api/goals", map[string]any{"internId": "u-anna", "title": title, "metric": "Есть результат", "due": "2026-12-20",
			"tasks": []map[string]string{{"title": "Сделать первую задачу"}}}, &out); code != 201 {
			t.Fatalf("add goal: %d", code)
		}
		return out.Goal.ID
	}
	g1, g2, g3 := add(maria, "Цель руководителя"), add(oleg, "Цель наставника"), add(maria, "Ещё цель")
	for _, c := range []*client{anna, e.login("sergey"), e.login("dmitry")} {
		if code := c.do("DELETE", "/api/goals/"+g1, nil, nil); code != 403 {
			t.Fatalf("foreign delete: %d", code)
		}
	}
	if code := maria.do("DELETE", "/api/goals/plan-w1", nil, nil); code != 400 {
		t.Fatalf("plan goal: %d", code)
	}
	for c, id := range map[*client]string{maria: g1, oleg: g2, admin: g3} {
		if code := c.do("DELETE", "/api/goals/"+id, nil, nil); code != 200 {
			t.Fatalf("delete %s: %d", id, code)
		}
	}
	if code := maria.do("DELETE", "/api/goals/"+g1, nil, nil); code != 404 {
		t.Fatal("twice")
	}
	var list struct{ Goals []domain.Goal }
	anna.do("GET", "/api/goals", nil, &list)
	if len(manual(list.Goals)) != 0 {
		t.Fatalf("left: %+v", manual(list.Goals))
	}
	var n struct{ Notifications []domain.Notification }
	anna.do("GET", "/api/notifications", nil, &n)
	if !strings.Contains(n.Notifications[0].Text, "убрал(а) цель") {
		t.Fatalf("notify: %+v", n.Notifications[0])
	}
	// длинный ответ наставника и длинный комментарий руководителя — без ограничения длины
	_, q := anna.ask("спроси наставника: где тестовый стенд поиска?")
	anna.do("POST", "/api/actions/"+q.ID+"/confirm", nil, nil)
	var inbox struct{ Questions []domain.Question }
	oleg.do("GET", "/api/inbox", nil, &inbox)
	long := strings.Repeat("Подробный ответ. ", 1000) // ~17 000 символов
	if code := oleg.do("POST", "/api/inbox/"+inbox.Questions[0].ID+"/answer", map[string]any{"answer": long}, nil); code != 200 {
		t.Fatalf("long answer: %d", code)
	}
	_, abs := anna.ask("хочу отгул 16 ноября")
	anna.do("POST", "/api/actions/"+abs.ID+"/confirm", nil, nil)
	var ap struct {
		Approvals []struct{ Kind, ExternalID string }
	}
	maria.do("GET", "/api/approvals", nil, &ap)
	if code := maria.do("POST", "/api/approvals/"+ap.Approvals[0].Kind+"/"+ap.Approvals[0].ExternalID,
		map[string]any{"approve": false, "comment": strings.Repeat("Причина. ", 300)}, nil); code != 200 {
		t.Fatalf("long comment: %d", code)
	}
}
