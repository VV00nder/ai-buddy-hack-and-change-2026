package api

import (
	"strings"
	"testing"

	"aibuddy/internal/app"
	"aibuddy/internal/domain"
)

func TestSeveralMentors(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	admin, maria, anna, oleg := e.login("admin"), e.login("maria"), e.login("anna"), e.login("oleg")
	// второй наставник в «Поиске»: администратор делает Павла наставником отдела
	if code := admin.do("PATCH", "/api/admin/users/u-pavel", map[string]string{"role": "mentor", "team": "search"}, nil); code != 200 {
		t.Fatalf("pavel mentor: %d", code)
	}
	for _, c := range []struct {
		who  *client
		ids  []string
		code int
	}{
		{anna, []string{"u-oleg"}, 403},
		{oleg, []string{"u-oleg", "u-pavel"}, 403},
		{e.login("sergey"), []string{"u-oleg"}, 403},
		{maria, []string{}, 400},
		{maria, []string{"u-dmitry"}, 400}, // наставник другого отдела — только администратор
		{maria, []string{"u-maria"}, 400},  // не наставник
	} {
		if code := c.who.do("PUT", "/api/team/u-anna/mentors", map[string]any{"mentorIds": c.ids}, nil); code != c.code {
			t.Fatalf("%v: %d", c.ids, code)
		}
	}
	var out struct{ User domain.User }
	if code := maria.do("PUT", "/api/team/u-anna/mentors", map[string]any{"mentorIds": []string{"u-oleg", "u-pavel"}}, &out); code != 200 ||
		out.User.MentorID != "u-oleg" || !out.User.HasMentor("u-pavel") {
		t.Fatalf("set mentors: %d %+v", code, out.User)
	}
	pavel := e.login("pavel")
	var team struct{ Interns []app.InternSummary }
	if pavel.do("GET", "/api/team", nil, &team); len(team.Interns) != 1 {
		t.Fatalf("second mentor sees intern: %+v", team)
	}
	var d app.Dashboard
	maria.do("GET", "/api/dashboard", nil, &d)
	if d.Interns[0].MentorName != "Олег Кузнецов, Павел Егоров" {
		t.Fatalf("mentor names: %q", d.Interns[0].MentorName)
	}
	// вопрос видят оба наставника, отвечает любой
	_, q := anna.ask("спроси наставника: где тестовый стенд поиска?")
	anna.do("POST", "/api/actions/"+q.ID+"/confirm", nil, nil)
	var inbox struct{ Questions []domain.Question }
	if pavel.do("GET", "/api/inbox", nil, &inbox); len(inbox.Questions) != 1 {
		t.Fatalf("pavel inbox: %+v", inbox)
	}
	var notes struct{ Notifications []domain.Notification }
	oleg.do("GET", "/api/notifications", nil, &notes)
	if !strings.Contains(notes.Notifications[0].Text, "спрашивает") {
		t.Fatalf("oleg notified: %+v", notes.Notifications[0])
	}
	if code := pavel.do("POST", "/api/inbox/"+inbox.Questions[0].ID+"/answer", map[string]any{"answer": "Стенд — search-staging"}, nil); code != 200 {
		t.Fatalf("second mentor answers: %d", code)
	}
	if oleg.do("GET", "/api/inbox", nil, &inbox); inbox.Questions[0].Status != "answered" || inbox.Questions[0].AnsweredBy != "Павел Егоров" {
		t.Fatalf("answered by: %+v", inbox.Questions[0])
	}
	// администратор может назначить наставника из другого отдела; руководитель и админ отвечают на вопросы
	if code := admin.do("PUT", "/api/team/u-anna/mentors", map[string]any{"mentorIds": []string{"u-oleg", "u-dmitry"}}, &out); code != 200 || !out.User.HasMentor("u-dmitry") {
		t.Fatalf("admin cross-team mentor: %d", code)
	}
	_, q2 := anna.ask("спроси наставника: как устроен релиз поиска?")
	anna.do("POST", "/api/actions/"+q2.ID+"/confirm", nil, nil)
	var rq struct {
		Questions []app.InternQuestion
		CanAnswer bool
	}
	if maria.do("GET", "/api/team/u-anna/requests", nil, &rq); !rq.CanAnswer || len(rq.Questions) != 2 || rq.Questions[0].Status != "open" {
		t.Fatalf("lead can answer: %+v", rq)
	}
	if code := maria.do("POST", "/api/inbox/"+rq.Questions[0].ID+"/answer", map[string]any{"answer": "Раз в неделю, по четвергам"}, nil); code != 200 {
		t.Fatalf("lead answers: %d", code)
	}
	var msgs struct{ Messages []domain.Message }
	anna.do("GET", "/api/messages", nil, &msgs)
	if last := msgs.Messages[len(msgs.Messages)-1]; !strings.Contains(last.Text, "Мария Волкова (руководитель)") {
		t.Fatalf("answer in chat: %s", last.Text)
	}
	_, q3 := anna.ask("спроси наставника: где логи сервиса?")
	anna.do("POST", "/api/actions/"+q3.ID+"/confirm", nil, nil)
	admin.do("GET", "/api/inbox", nil, &inbox)
	if code := admin.do("POST", "/api/inbox/"+inbox.Questions[0].ID+"/answer", map[string]any{"answer": "В Observer"}, nil); code != 200 {
		t.Fatalf("admin answers: %d", code)
	}
}
