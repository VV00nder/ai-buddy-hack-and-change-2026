package api

import (
	"slices"
	"strings"
	"testing"

	"aibuddy/internal/app"
	"aibuddy/internal/domain"
)

func TestAdmin(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	admin, anna, ivan, maria := e.login("admin"), e.login("anna"), e.login("ivan"), e.login("maria")
	if p := perms(admin); !slices.Contains(p, "admin") || !slices.Contains(p, "approvals") || !slices.Contains(p, "kb") || !slices.Contains(p, "programs") {
		t.Fatalf("admin perms: %v", p)
	}
	// служебные разделы — только администратору
	for _, c := range []*client{anna, maria} {
		if code := c.do("GET", "/api/admin/service", nil, nil); code != 403 {
			t.Fatal("service for non-admin")
		}
		if code := c.do("PATCH", "/api/admin/users/u-ivan", map[string]string{"role": "mentor", "team": "delivery"}, nil); code != 403 {
			t.Fatal("set user for non-admin")
		}
	}
	// стажёры обоих отделов и их заявки — у администратора
	_, abs := anna.ask("хочу отгул 16 ноября")
	anna.do("POST", "/api/actions/"+abs.ID+"/confirm", nil, nil)
	_, abs2 := ivan.ask("хочу отгул 17 ноября")
	ivan.do("POST", "/api/actions/"+abs2.ID+"/confirm", nil, nil)
	var d app.Dashboard
	if code := admin.do("GET", "/api/dashboard", nil, &d); code != 200 || len(d.Interns) != 2 || len(d.Approvals) != 2 {
		t.Fatalf("admin dashboard: %d %+v", code, d)
	}
	// заявка в карточке стажёра сразу с данными для решения
	var rq struct{ Requests []app.InternRequest }
	admin.do("GET", "/api/team/u-ivan/requests", nil, &rq)
	if len(rq.Requests) != 1 || !rq.Requests[0].AwaitingMe || rq.Requests[0].Approval == nil {
		t.Fatalf("requests: %+v", rq)
	}
	ap := rq.Requests[0].Approval
	if code := admin.do("POST", "/api/approvals/"+ap.Kind+"/"+ap.ExternalID, map[string]any{"approve": false, "comment": "В этот день релиз"}, nil); code != 200 {
		t.Fatalf("admin reject: %d", code)
	}
	var acts struct{ Actions []domain.Action }
	ivan.do("GET", "/api/actions", nil, &acts)
	if acts.Actions[0].Status != domain.ActionRejected {
		t.Fatalf("ivan request: %+v", acts.Actions[0])
	}
	// план любого стажёра: отметить шаг и поставить цель
	var plan struct {
		Periods []struct{ Steps []struct{ ID string } }
	}
	if code := admin.do("GET", "/api/team/u-ivan/plan", nil, &plan); code != 200 {
		t.Fatalf("plan: %d", code)
	}
	// все сотрудники в «Отделе», смена роли и отдела
	var dep app.Department
	if code := admin.do("GET", "/api/department", nil, &dep); code != 200 || len(dep.Members) < 8 {
		t.Fatalf("department: %d %d", code, len(dep.Members))
	}
	if len(dep.Teams) != 3 || len(dep.Mentors) != 3 {
		t.Fatalf("admin sees all teams (incl. without lead) and mentors: %+v %+v", dep.Teams, dep.Mentors)
	}
	var errResp struct{ Message string }
	if code := admin.do("PATCH", "/api/admin/users/u-oleg", map[string]string{"role": "lead", "team": "search"}, &errResp); code != 400 || !strings.Contains(errResp.Message, "Мария Волкова") {
		t.Fatalf("second lead: %d %+v", code, errResp)
	}
	if code := admin.do("PATCH", "/api/admin/users/u-ivan", map[string]string{"role": "intern", "team": "search", "mentorId": "u-oleg"}, &dep); code != 200 {
		t.Fatalf("move ivan: %d", code)
	}
	ivanNow := e.login("ivan")
	var me struct{ User domain.User }
	ivanNow.do("GET", "/api/me", nil, &me)
	if me.User.Team != "search" || me.User.LeadID != "u-maria" || me.User.MentorID != "u-oleg" {
		t.Fatalf("ivan moved: %+v", me.User)
	}
	var notes struct{ Notifications []domain.Notification }
	ivanNow.do("GET", "/api/notifications", nil, &notes)
	if !strings.Contains(notes.Notifications[0].Text, "назначил(а) вам роль «стажёр»") {
		t.Fatalf("notify: %+v", notes.Notifications[0])
	}
	// новичка принимает в любой отдел
	c := e.anon()
	c.do("POST", "/api/auth/register", map[string]string{"name": "Пётр Сидоров", "email": "petr@example.com", "password": "secret123"}, nil)
	admin.do("GET", "/api/department", nil, &dep)
	if len(dep.Newcomers) != 1 {
		t.Fatalf("newcomers: %+v", dep.Newcomers)
	}
	if code := admin.do("POST", "/api/department/"+dep.Newcomers[0].ID+"/assign", map[string]string{"role": "engineer", "team": "delivery"}, &dep); code != 200 || len(dep.Newcomers) != 0 {
		t.Fatalf("admin assign: %d", code)
	}
	// сброс пароля
	if code := admin.do("POST", "/api/admin/users/u-anna/password", map[string]string{"password": "short"}, nil); code != 400 {
		t.Fatal("weak password")
	}
	if code := admin.do("POST", "/api/admin/users/u-anna/password", map[string]string{"password": "newpass2026"}, nil); code != 200 {
		t.Fatal("reset")
	}
	if code := e.anon().do("POST", "/api/auth/login", map[string]string{"login": "anna", "password": "newpass2026"}, nil); code != 200 {
		t.Fatal("login with new password")
	}
	// права сотрудников всех отделов
	var rights struct{ Members []app.Member }
	if code := admin.do("GET", "/api/rights", nil, &rights); code != 200 || len(rights.Members) < 4 {
		t.Fatalf("rights: %d %+v", code, rights)
	}
	if code := admin.do("POST", "/api/rights", map[string]any{"userId": "u-dmitry", "perm": "approvals", "enabled": true}, &rights); code != 200 {
		t.Fatalf("grant: %d", code)
	}
	// общая инструкция администратора видна всем отделам, руководитель её не правит
	doc := map[string]any{"title": "Политика паролей", "folder": "Безопасность", "audience": []string{"all"}, "body": "## Пароли\n\nНе короче 12 символов и не повторяются между сервисами."}
	var created docView
	if code := admin.do("POST", "/api/kb", doc, &created); code != 201 {
		t.Fatalf("admin doc: %d", code)
	}
	var list kbResp
	e.login("ivan").do("GET", "/api/kb", nil, &list)
	if !slices.ContainsFunc(list.Docs, func(d docView) bool { return d.ID == created.ID }) {
		t.Fatal("company doc visible to another team")
	}
	if code := maria.do("PUT", "/api/kb/"+created.ID, doc, nil); code != 403 {
		t.Fatalf("lead edits company doc: %d", code)
	}
	// флаги: выключенные заявки на отгул перестают создаваться
	var svc app.Service
	if code := admin.do("GET", "/api/admin/service", nil, &svc); code != 200 || svc.Status["store"] != "ok" || svc.Roles["intern"] < 2 || len(svc.Audit) == 0 || svc.Audit[0].UserName == "" {
		t.Fatalf("service: %d %+v", code, svc)
	}
	if code := admin.do("PUT", "/api/admin/flags/llm", map[string]bool{"enabled": false}, nil); code != 400 {
		t.Fatal("llm flag is start-only")
	}
	if code := admin.do("PUT", "/api/admin/flags/absenceActions", map[string]bool{"enabled": false}, nil); code != 200 || e.app.Flags().AbsenceActions {
		t.Fatal("flag off")
	}
	if err := e.app.LoadFlags(t.Context()); err != nil || e.app.Flags().AbsenceActions {
		t.Fatal("flag persisted")
	}
	admin.do("PUT", "/api/admin/flags/absenceActions", map[string]bool{"enabled": true}, nil)
	if code := admin.do("POST", "/api/admin/cron", nil, nil); code != 200 {
		t.Fatalf("cron: %d", code)
	}
}
