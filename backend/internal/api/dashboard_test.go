package api

import (
	"strings"
	"testing"
	"time"

	"aibuddy/internal/app"
	"aibuddy/internal/domain"
)

func TestLeadDashboard(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna, maria, oleg := e.login("anna"), e.login("maria"), e.login("oleg")
	// Анна просит отгул и доступ вне правил — оба ждут Марию
	_, abs := anna.ask("хочу отгул 16 ноября")
	anna.do("POST", "/api/actions/"+abs.ID+"/confirm", nil, nil)
	anna.ask("нужен админский доступ к stash search-indexer")
	_, exc := anna.ask("Нужно настроить индексацию для задачи SEARCH-90")
	if exc == nil {
		t.Fatal("exception proposal")
	}
	anna.do("POST", "/api/actions/"+exc.ID+"/confirm", nil, nil)
	// вопрос наставнику — не согласование: в заявки стажёра не попадает
	if _, q := anna.ask("спроси наставника: где тестовый стенд?"); q != nil {
		anna.do("POST", "/api/actions/"+q.ID+"/confirm", nil, nil)
	}
	// время идёт: заявки висят больше суток
	e.clock = e.clock.Add(30 * time.Hour)
	maria, anna, oleg = e.login("maria"), e.login("anna"), e.login("oleg")
	var d app.Dashboard
	if code := maria.do("GET", "/api/dashboard", nil, &d); code != 200 {
		t.Fatalf("dashboard: %d", code)
	}
	if len(d.Approvals) != 2 || d.Approvals[0].WaitingHours < 24 {
		t.Fatalf("approvals: %+v", d.Approvals)
	}
	exception := false
	for _, ap := range d.Approvals {
		exception = exception || ap.Details["exception"] != nil
	}
	if !exception {
		t.Fatal("exception flag visible to lead")
	}
	if len(d.Interns) != 1 || d.Interns[0].PendingApprovals != 2 || d.Interns[0].MentorName != "Олег Кузнецов" {
		t.Fatalf("intern card: %+v", d.Interns)
	}
	alerts := strings.Join(d.Interns[0].Alerts, "; ")
	if !strings.Contains(alerts, "ждёт вашего решения: 2 (уже 30 ч)") {
		t.Fatalf("alerts: %s", alerts)
	}
	// заявки стажёра в карточке: какие просил и какие ждут меня
	var rq struct{ Requests []app.InternRequest }
	if code := maria.do("GET", "/api/team/u-anna/requests", nil, &rq); code != 200 || len(rq.Requests) != 2 || !rq.Requests[0].AwaitingMe {
		t.Fatalf("requests: %d %+v", code, rq)
	}
	if code := e.login("dmitry").do("GET", "/api/team/u-anna/requests", nil, nil); code != 403 {
		t.Fatal("other mentor requests")
	}
	if code := anna.do("GET", "/api/dashboard", nil, nil); code != 403 {
		t.Fatal("intern dashboard")
	}
	var od app.Dashboard
	if code := oleg.do("GET", "/api/dashboard", nil, &od); code != 200 || len(od.Interns) != 1 || od.Interns[0].PendingApprovals != 0 {
		t.Fatalf("mentor dashboard: %+v", od)
	}
	// напоминание по заявке
	var rm app.Reminder
	if code := maria.do("POST", "/api/reminders", map[string]any{"text": "Решить по отгулу Анны", "link": "/approvals", "inHours": 2}, &rm); code != 201 {
		t.Fatalf("reminder: %d", code)
	}
	for _, bad := range []map[string]any{{"text": "x", "inHours": 1}, {"text": "Нормально"}, {"text": "Нормально", "at": "2020-01-01T10:00"}, {"text": "Нормально", "at": "завтра"}} {
		if code := maria.do("POST", "/api/reminders", bad, nil); code != 400 {
			t.Fatalf("bad reminder %v: %d", bad, code)
		}
	}
	at := e.clock.Add(26 * time.Hour).Format("2006-01-02T15:04")
	if code := maria.do("POST", "/api/reminders", map[string]any{"text": "Проверить план Анны", "at": at}, nil); code != 201 {
		t.Fatal("reminder at")
	}
	maria.do("GET", "/api/dashboard", nil, &d)
	if len(d.Reminders) != 2 || d.Reminders[0].ID != rm.ID {
		t.Fatalf("reminders in dashboard: %+v", d.Reminders)
	}
	if code := oleg.do("DELETE", "/api/reminders/"+rm.ID, nil, nil); code != 404 {
		t.Fatal("foreign reminder delete")
	}
	e.clock = e.clock.Add(3 * time.Hour)
	maria = e.login("maria")
	if n := e.app.FireReminders(t.Context()); n != 1 {
		t.Fatalf("fired: %d", n)
	}
	var notes struct{ Notifications []domain.Notification }
	maria.do("GET", "/api/notifications", nil, &notes)
	if !strings.Contains(notes.Notifications[0].Text, "Напоминание: Решить по отгулу Анны") {
		t.Fatalf("reminder notification: %+v", notes.Notifications[0])
	}
	var list struct{ Reminders []app.Reminder }
	maria.do("GET", "/api/reminders", nil, &list)
	if len(list.Reminders) != 1 {
		t.Fatal("sent reminder hidden")
	}
	if code := maria.do("DELETE", "/api/reminders/"+list.Reminders[0].ID, nil, nil); code != 200 {
		t.Fatal("delete reminder")
	}
	if code := maria.do("DELETE", "/api/reminders/none", nil, nil); code != 404 {
		t.Fatal("delete missing")
	}
	// через две недели первые шаги просрочены — отметка «отстаёт от плана»
	e.clock = e.clock.AddDate(0, 0, 14)
	e.login("maria").do("GET", "/api/dashboard", nil, &d)
	if a := strings.Join(d.Interns[0].Alerts, "; "); !strings.Contains(a, "отстаёт от плана") || !strings.Contains(a, "дн)") {
		t.Fatalf("overdue alert: %s", a)
	}
}
