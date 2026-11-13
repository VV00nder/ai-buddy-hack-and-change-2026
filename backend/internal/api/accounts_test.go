package api

import (
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"aibuddy/internal/app"
	"aibuddy/internal/domain"
	"aibuddy/internal/onboarding"
)

func (e *testEnv) anon() *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: e.t, e: e, hc: &http.Client{Jar: jar}}
}

func TestRegistration(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	c := e.anon()
	var teams struct{ Teams []app.TeamInfo }
	if code := c.do("GET", "/api/auth/teams", nil, &teams); code != 200 || len(teams.Teams) != 2 {
		t.Fatalf("teams: %d %+v", code, teams)
	}
	var search app.TeamInfo
	for _, tm := range teams.Teams {
		if tm.ID == "search" {
			search = tm
		}
	}
	if search.LeadName != "Мария Волкова" {
		t.Fatalf("lead in team list: %+v", search)
	}
	in := map[string]string{"name": "Пётр Сидоров", "email": "petr@example.com", "password": "secret123"}
	bad := map[string]string{"name": "Пётр", "email": "petr@example.com", "password": "secret123"}
	var errResp struct{ Message string }
	if code := c.do("POST", "/api/auth/register", bad, &errResp); code != 400 || !strings.Contains(errResp.Message, "фамилию") {
		t.Fatalf("validation: %d %+v", code, errResp)
	}
	var reg struct{ User domain.User }
	if code := c.do("POST", "/api/auth/register", in, &reg); code != 201 || reg.User.Role != domain.RoleNewcomer || reg.User.Team != "" {
		t.Fatalf("register: %d %+v", code, reg)
	}
	// до назначения в отдел плана нет
	if code := c.do("GET", "/api/plan", nil, nil); code == 200 {
		t.Fatal("newcomer has plan")
	}
	// руководитель видит нового сотрудника и принимает его стажёром
	maria := e.login("maria")
	var notes0 struct{ Notifications []domain.Notification }
	maria.do("GET", "/api/notifications", nil, &notes0)
	if len(notes0.Notifications) == 0 || !strings.Contains(notes0.Notifications[0].Text, "ждёт назначения") {
		t.Fatalf("lead notified: %+v", notes0)
	}
	var dep app.Department
	maria.do("GET", "/api/department", nil, &dep)
	if len(dep.Newcomers) != 1 || len(dep.Mentors) != 2 || dep.Mentors[0].ID != "u-oleg" {
		t.Fatalf("department newcomers: %+v", dep)
	}
	if code := e.login("oleg").do("POST", "/api/department/"+reg.User.ID+"/assign", map[string]string{"role": "intern"}, nil); code != 403 {
		t.Fatal("mentor assigns")
	}
	if code := maria.do("POST", "/api/department/"+reg.User.ID+"/assign", map[string]string{"role": "boss"}, nil); code != 400 {
		t.Fatal("bad role")
	}
	if code := maria.do("POST", "/api/department/"+reg.User.ID+"/assign", map[string]string{"role": "intern", "mentorId": "u-oleg"}, &dep); code != 200 || len(dep.Newcomers) != 0 {
		t.Fatalf("assign: %d %+v", code, dep)
	}
	// сразу (без повторного входа): план стажёра с первого дня
	var plan onboarding.Plan
	if code := c.do("GET", "/api/plan", nil, &plan); code != 200 || plan.Day != 1 {
		t.Fatalf("plan of new intern: %d %+v", code, plan.Day)
	}
	if code := e.anon().do("POST", "/api/auth/register", in, &errResp); code != 400 || !strings.Contains(errResp.Message, "уже зарегистрирована") {
		t.Fatal("duplicate email")
	}
	// вход по почте и паролю; тестовый пароль не подходит
	c2 := e.anon()
	if code := c2.do("POST", "/api/auth/login", map[string]string{"login": "Petr@Example.com", "password": "secret123"}, nil); code != 200 {
		t.Fatal("login by email")
	}
	if code := e.anon().do("POST", "/api/auth/login", map[string]string{"login": "petr@example.com", "password": "buddy2026"}, nil); code != 401 {
		t.Fatal("demo password must not work")
	}
	// наставник видит нового стажёра
	oleg := e.login("oleg")
	var team struct{ Interns []app.InternSummary }
	oleg.do("GET", "/api/team", nil, &team)
	if len(team.Interns) != 2 {
		t.Fatalf("mentor sees new intern: %+v", team.Interns)
	}
	var notes struct{ Notifications []domain.Notification }
	c.do("GET", "/api/notifications", nil, &notes)
	if len(notes.Notifications) == 0 || !strings.Contains(notes.Notifications[0].Text, "Мария") {
		t.Fatalf("newcomer notified: %+v", notes)
	}
	// на экране входа — только тестовые учётные записи
	var demo struct{ Users []domain.User }
	e.anon().do("GET", "/api/auth/demo-users", nil, &demo)
	for _, u := range demo.Users {
		if u.Registered {
			t.Fatal("registered user on login screen")
		}
	}
	// ограничение частоты регистраций
	for i := 0; i < 6; i++ {
		e.anon().do("POST", "/api/auth/register", bad, nil)
	}
	if code := e.anon().do("POST", "/api/auth/register", bad, nil); code != 429 {
		t.Fatalf("rate limit: %d", code)
	}
	e.corp.SetOutage(true)
	if code := e.anon().do("GET", "/api/auth/teams", nil, nil); code != 200 {
		t.Log("teams are served by SSO even during MCP outage")
	}
}

func TestTransfer(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	maria, anna, oleg, sergey, dmitry := e.login("maria"), e.login("anna"), e.login("oleg"), e.login("sergey"), e.login("dmitry")
	var dep app.Department
	if code := oleg.do("GET", "/api/department", nil, nil); code != 403 {
		t.Fatal("mentor department")
	}
	if code := maria.do("GET", "/api/department", nil, &dep); code != 200 || len(dep.Members) != 4 || len(dep.Teams) != 2 {
		t.Fatalf("department: %d %+v", code, dep)
	}
	// заявка Анны ждёт Марию — после перевода перейдёт Сергею
	_, act := anna.ask("хочу отгул 16 ноября")
	anna.do("POST", "/api/actions/"+act.ID+"/confirm", nil, nil)
	for path, code := range map[string]int{"/api/department/u-ivan/transfer": 400, "/api/department/u-maria/transfer": 400} {
		if got := maria.do("POST", path, map[string]string{"team": "delivery"}, nil); got != code {
			t.Fatalf("%s: %d", path, got)
		}
	}
	if code := oleg.do("POST", "/api/department/u-anna/transfer", map[string]string{"team": "delivery"}, nil); code != 403 {
		t.Fatal("mentor transfers")
	}
	if code := maria.do("POST", "/api/department/u-anna/transfer", map[string]string{"team": "delivery"}, &dep); code != 200 || len(dep.Members) != 3 {
		t.Fatalf("transfer: %d %+v", code, dep)
	}
	// у Анны новый отдел, руководитель и наставник — без повторного входа
	var me struct{ User domain.User }
	anna.do("GET", "/api/me", nil, &me)
	if me.User.Team != "delivery" || me.User.LeadID != "u-sergey" || me.User.MentorID != "u-dmitry" {
		t.Fatalf("me after transfer: %+v", me.User)
	}
	var notes struct{ Notifications []domain.Notification }
	anna.do("GET", "/api/notifications", nil, &notes)
	if !strings.Contains(notes.Notifications[0].Text, "Сергей") {
		t.Fatalf("intern notified: %+v", notes.Notifications[0])
	}
	// новый руководитель видит стажёра и её заявку, старый — нет
	var ap struct{ Approvals []app.Approval }
	sergey.do("GET", "/api/approvals", nil, &ap)
	if len(ap.Approvals) != 1 {
		t.Fatalf("new lead approvals: %+v", ap)
	}
	maria.do("GET", "/api/approvals", nil, &ap)
	if len(ap.Approvals) != 0 {
		t.Fatal("old lead approvals")
	}
	var team struct{ Interns []app.InternSummary }
	dmitry.do("GET", "/api/team", nil, &team)
	if len(team.Interns) != 2 {
		t.Fatalf("new mentor team: %+v", team.Interns)
	}
	if code := oleg.do("GET", "/api/team/u-anna/plan", nil, nil); code != 403 {
		t.Fatal("old mentor still sees plan")
	}
	// агент знает новый отдел (токен старый, справочник новый)
	if m, _ := anna.ask("кто мой наставник?"); !strings.Contains(m.Text, "Дмитр") {
		t.Fatalf("whoami after transfer: %s", m.Text)
	}
	e.corp.SetOutage(true)
	if code := sergey.do("POST", "/api/department/u-anna/transfer", map[string]string{"team": "search"}, nil); code != 503 {
		t.Fatal("outage")
	}
}

func TestLeadAsksAboutInterns(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	m, _ := e.login("maria").ask("кто сейчас у нас стажируется")
	if !strings.Contains(m.Text, "Анна Смирнова") || !strings.Contains(m.Text, "4-й день") || strings.Contains(m.Text, "Иван") {
		t.Fatalf("lead: %s", m.Text)
	}
	if m, _ := e.login("oleg").ask("как дела у моих стажёров?"); !strings.Contains(m.Text, "Анна Смирнова") {
		t.Fatalf("mentor: %s", m.Text)
	}
	if m, _ := e.login("pavel").ask("кто стажируется?"); !strings.Contains(m.Text, "нет стажёров") {
		t.Fatalf("engineer: %s", m.Text)
	}
	// стажёр не видит других стажёров
	if m, _ := e.login("anna").ask("кто ещё стажируется?"); strings.Contains(m.Text, "Иван") {
		t.Fatalf("intern: %s", m.Text)
	}
}
