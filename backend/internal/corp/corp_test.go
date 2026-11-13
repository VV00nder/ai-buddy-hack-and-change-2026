package corp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aibuddy/internal/identity"
	"aibuddy/internal/mcp"
)

var msk = time.FixedZone("MSK", 3*3600)

// Вторник, 10 ноября 2026.
func fixedNow() time.Time { return time.Date(2026, 11, 10, 12, 0, 0, 0, msk) }

func newSvc(t *testing.T) *Service {
	t.Helper()
	s, err := New(fixedNow, msk)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func claims(s *Service, login string) identity.Claims {
	u, _ := s.UserByLogin(login)
	return identity.Claims{Sub: u.ID, Login: u.Login, Role: u.Role, Team: u.Team}
}

func toolCode(err error) string {
	var te *mcp.ToolError
	if errors.As(err, &te) {
		return te.Code
	}
	if errors.Is(err, mcp.ErrForbidden) {
		return "forbidden"
	}
	if errors.Is(err, errOutage) {
		return "outage"
	}
	return ""
}

func TestDirectory(t *testing.T) {
	s := newSvc(t)
	if len(s.Users()) != 8 {
		t.Fatal("users")
	}
	anna := claims(s, "anna")
	who, err := s.WhoAmI(anna)
	if err != nil || who["mentor"].(map[string]any)["name"] != "Олег Кузнецов" || who["lead"] == nil {
		t.Fatalf("whoami: %v %v", who, err)
	}
	u, _ := s.UserByLogin("anna")
	if u.StartDate.Format(dateLayout) != "2026-11-07" || u.TeamName != "Поиск объявлений" {
		t.Fatalf("start date: %v", u.StartDate)
	}
	if _, ok := s.UserByLogin("nobody"); ok {
		t.Fatal("nobody")
	}
	if _, err := s.WhoAmI(identity.Claims{Sub: "x"}); toolCode(err) != "unknown_user" {
		t.Fatal(err)
	}
	team, _ := s.TeamMembers(anna)
	if len(team["members"].([]map[string]any)) != 4 { // anna, oleg, maria, elena
		t.Fatalf("team: %v", team["members"])
	}
	if AbsencePolicy()["types"] == nil {
		t.Fatal("policy")
	}
}

func TestAbsenceRules(t *testing.T) {
	s := newSvc(t)
	anna := claims(s, "anna")
	cases := []struct {
		in   AbsenceInput
		code string
	}{
		{AbsenceInput{DateFrom: "2026-11-11"}, "too_late"},  // завтра — меньше 2 рабочих дней
		{AbsenceInput{DateFrom: "2026-11-10"}, "past_date"}, // сегодня
		{AbsenceInput{DateFrom: "2026-11-14"}, "weekend"},   // суббота
		{AbsenceInput{DateFrom: "bad"}, "bad_dates"},        // формат
		{AbsenceInput{DateFrom: "2026-11-20", DateTo: "2026-11-19"}, "bad_dates"},
		{AbsenceInput{DateFrom: "2026-11-16", DateTo: "2026-11-19"}, "too_long"}, // 4 рабочих дня
		{AbsenceInput{Type: "sick", DateFrom: "2026-11-16"}, "bad_type"},
		{AbsenceInput{Type: "vacation", DateFrom: "2026-12-16"}, "not_eligible"},
	}
	for _, c := range cases {
		if _, err := s.CreateAbsence(anna, c.in); toolCode(err) != c.code {
			t.Errorf("%+v: want %s, got %v", c.in, c.code, err)
		}
	}
	// dry-run: проверка без создания
	prev, err := s.CreateAbsence(anna, AbsenceInput{DateFrom: "2026-11-13", Reason: "учёба", DryRun: true})
	if err != nil || prev["status"] != "valid" || prev["approver"] != "Мария Волкова" || prev["balance_after"] != 2 {
		t.Fatalf("dry run: %v %v", prev, err)
	}
	if l, _ := s.ListAbsences(anna, "mine"); len(l["items"].([]Absence)) != 0 {
		t.Fatal("dry run must not create")
	}
	res, err := s.CreateAbsence(anna, AbsenceInput{DateFrom: "2026-11-13", DateTo: "2026-11-13"})
	if err != nil || res["status"] != "pending" || res["id"] == "" {
		t.Fatalf("create: %v %v", res, err)
	}
	id := res["id"].(string)
	if _, err := s.CreateAbsence(anna, AbsenceInput{DateFrom: "2026-11-13"}); toolCode(err) != "overlap" {
		t.Fatalf("overlap: %v", err)
	}
	if _, err := s.CreateAbsence(anna, AbsenceInput{DateFrom: "2026-11-16", DateTo: "2026-11-18"}); toolCode(err) != "no_balance" {
		t.Fatalf("balance: %v", err)
	}
	bal, _ := s.AbsenceBalance(anna)
	if bal["day_off_left"] != 2 {
		t.Fatalf("balance: %v", bal)
	}
	// права: Иван (другая команда) и сам стажёр не могут согласовать; Сергей — чужой руководитель
	if _, err := s.GetAbsence(claims(s, "ivan"), id); toolCode(err) != "forbidden" {
		t.Fatal("foreign get")
	}
	if _, err := s.DecideAbsence(claims(s, "sergey"), id, "approved", ""); toolCode(err) != "forbidden" {
		t.Fatal("foreign lead")
	}
	maria := claims(s, "maria")
	if l, _ := s.ListAbsences(maria, "approvals"); len(l["items"].([]Absence)) != 1 {
		t.Fatal("approvals")
	}
	if _, err := s.DecideAbsence(maria, id, "maybe", ""); toolCode(err) != "bad_decision" {
		t.Fatal("bad decision")
	}
	if _, err := s.DecideAbsence(maria, "ABS-x", "approved", ""); toolCode(err) != "not_found" {
		t.Fatal("not found")
	}
	got, err := s.DecideAbsence(maria, id, "rejected", "Релиз в пятницу")
	if err != nil || got["status"] != "rejected" {
		t.Fatalf("decide: %v %v", got, err)
	}
	if bal, _ := s.AbsenceBalance(anna); bal["day_off_left"] != 3 {
		t.Fatalf("balance must be returned: %v", bal)
	}
	if _, err := s.DecideAbsence(maria, id, "approved", ""); toolCode(err) != "already_decided" {
		t.Fatal("already decided")
	}
	if g, err := s.GetAbsence(anna, id); err != nil || g["comment"] != "Релиз в пятницу" {
		t.Fatalf("get: %v %v", g, err)
	}
	if _, err := s.GetAbsence(anna, "none"); toolCode(err) != "not_found" {
		t.Fatal("get not found")
	}
	// после отклонения можно подать снова на те же даты
	if _, err := s.CreateAbsence(anna, AbsenceInput{DateFrom: "2026-11-13"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAbsence(identity.Claims{Sub: "x"}, AbsenceInput{DateFrom: "2026-11-13"}); toolCode(err) != "unknown_user" {
		t.Fatal("unknown")
	}
	// отпуск у опытного сотрудника
	oleg := claims(s, "oleg")
	if _, err := s.CreateAbsence(oleg, AbsenceInput{Type: "vacation", DateFrom: "2026-11-16"}); toolCode(err) != "too_late" {
		t.Fatal("vacation notice")
	}
	if r, err := s.CreateAbsence(oleg, AbsenceInput{Type: "vacation", DateFrom: "2026-12-14", DateTo: "2026-12-18"}); err != nil || r["days"] != 5 {
		t.Fatalf("vacation: %v %v", r, err)
	}
	if _, err := s.CreateAbsence(claims(s, "maria"), AbsenceInput{DateFrom: "2026-11-16"}); toolCode(err) != "no_approver" {
		t.Fatal("no approver")
	}
}

func TestAccess(t *testing.T) {
	s := newSvc(t)
	anna := claims(s, "anna")
	cat, _ := s.Catalog(anna)
	items := cat["items"].([]map[string]any)
	if len(items) != 11 {
		t.Fatalf("catalog: %d", len(items))
	}
	for _, it := range items {
		switch it["id"] {
		case "k8s-prod":
			if it["available"] != false {
				t.Fatal("k8s-prod must be unavailable for intern")
			}
		case "stash":
			if it["approverRole"] != "владелец проекта" || len(it["projects"].([]string)) != 3 {
				t.Fatalf("stash: %v", it)
			}
		}
	}
	cases := map[string]AccessInput{
		"unknown_system":         {System: "nope"},
		"not_allowed_for_role":   {System: "k8s-prod"},
		"bad_level":              {System: "vpn", Level: "admin"},
		"level_not_allowed":      {System: "stash", Project: "search-api", Level: "admin"},
		"project_required":       {System: "stash", Level: "edit"},
		"unknown_project":        {System: "stash", Project: "delivery-tracking"},
		"justification_required": {System: "redash", Justification: "надо"},
	}
	for code, in := range cases {
		if _, err := s.RequestAccess(anna, in); toolCode(err) != code {
			t.Errorf("%s: got %v", code, err)
		}
	}
	if _, err := s.RequestAccess(identity.Claims{Sub: "x"}, AccessInput{System: "vpn"}); toolCode(err) != "unknown_user" {
		t.Fatal("unknown user")
	}
	r, err := s.RequestAccess(anna, AccessInput{System: "grafana", Project: "ignored"})
	if err != nil || r["status"] != "granted" || r["project"] != nil {
		t.Fatalf("auto: %v %v", r, err)
	}
	if _, err := s.RequestAccess(anna, AccessInput{System: "grafana"}); toolCode(err) != "duplicate" {
		t.Fatal("dup granted")
	}
	// Stash: согласует владелец проекта
	dry, err := s.RequestAccess(anna, AccessInput{System: "stash", Project: "Search-API", Level: "edit", DryRun: true})
	if err != nil || dry["status"] != "valid" || dry["approver"] != "Олег Кузнецов" || dry["approver_role"] != "владелец проекта" || dry["level_name"] != "редактирование" {
		t.Fatalf("dry: %v %v", dry, err)
	}
	r, err = s.RequestAccess(anna, AccessInput{System: "stash", Project: "search-api", Level: "edit"})
	if err != nil || r["status"] != "pending" {
		t.Fatalf("pending: %v %v", r, err)
	}
	id := r["id"].(string)
	if _, err := s.RequestAccess(anna, AccessInput{System: "stash", Project: "search-api"}); toolCode(err) != "duplicate" {
		t.Fatal("dup pending same project")
	}
	// другой проект — отдельная заявка, владелец другой
	if r2, err := s.RequestAccess(anna, AccessInput{System: "stash", Project: "search-web", DryRun: true}); err != nil || r2["approver"] != "Мария Волкова" {
		t.Fatalf("other project owner: %v %v", r2, err)
	}
	// Kubernetes staging — дежурный DevOps
	k, err := s.RequestAccess(anna, AccessInput{System: "k8s-staging", Level: "edit", Justification: "Отладка индексатора поиска на стенде", DryRun: true})
	if err != nil || k["approver"] != "Павел Егоров" || k["approver_role"] != "дежурный DevOps" {
		t.Fatalf("devops: %v %v", k, err)
	}
	cat, _ = s.Catalog(anna)
	for _, it := range cat["items"].([]map[string]any) {
		if it["id"] == "stash" && it["myStatus"] != "pending" {
			t.Fatal("my status")
		}
	}
	oleg, maria := claims(s, "oleg"), claims(s, "maria")
	if l, _ := s.ListAccess(oleg, "approvals"); len(l["items"].([]AccessRequest)) != 1 {
		t.Fatal("owner approvals")
	}
	if l, _ := s.ListAccess(maria, "approvals"); len(l["items"].([]AccessRequest)) != 0 {
		t.Fatal("lead must not see owner's approvals")
	}
	if _, err := s.DecideAccess(maria, id, "granted", ""); toolCode(err) != "forbidden" {
		t.Fatal("not the approver")
	}
	if _, err := s.DecideAccess(oleg, id, "ok", ""); toolCode(err) != "bad_decision" {
		t.Fatal("bad decision")
	}
	if _, err := s.DecideAccess(oleg, "none", "granted", ""); toolCode(err) != "not_found" {
		t.Fatal("not found")
	}
	if d, err := s.DecideAccess(oleg, id, "granted", "ok"); err != nil || d["status"] != "granted" {
		t.Fatalf("decide: %v %v", d, err)
	}
	if _, err := s.DecideAccess(oleg, id, "granted", ""); toolCode(err) != "already_decided" {
		t.Fatal("already")
	}
	if l, _ := s.ListAccess(anna, "mine"); len(l["items"].([]AccessRequest)) != 2 {
		t.Fatal("mine")
	}
	if _, err := s.RequestAccess(anna, AccessInput{System: "redash", Justification: "Посчитать метрики поиска для задачи SEARCH-101"}); err != nil {
		t.Fatal(err)
	}
	// руководитель не может получить admin без обоснования
	if _, err := s.RequestAccess(maria, AccessInput{System: "stash", Project: "search-api", Level: "admin"}); toolCode(err) != "justification_required" {
		t.Fatal("admin needs justification")
	}
	// без владельца проекта согласует руководитель
	s.owners = map[string]string{}
	if r, _ := s.RequestAccess(anna, AccessInput{System: "stash", Project: "search-indexer", DryRun: true}); r["approver"] != "Мария Волкова" {
		t.Fatalf("fallback to lead: %v", r)
	}
	s.devops = "nobody"
	if _, err := s.RequestAccess(anna, AccessInput{System: "k8s-staging", Justification: "Отладка индексатора поиска на стенде"}); toolCode(err) != "no_approver" {
		t.Fatal("no approver")
	}
	if levelsRu([]string{"read", "edit"}) != "чтение (read), редактирование (edit)" {
		t.Fatal(levelsRu([]string{"read", "edit"}))
	}
}

func TestOutage(t *testing.T) {
	s := newSvc(t)
	s.SetOutage(true)
	c := claims(s, "anna")
	errs := []error{}
	_, e := s.CreateAbsence(c, AbsenceInput{})
	errs = append(errs, e)
	_, e = s.AbsenceBalance(c)
	errs = append(errs, e)
	_, e = s.ListAbsences(c, "")
	errs = append(errs, e)
	_, e = s.GetAbsence(c, "")
	errs = append(errs, e)
	_, e = s.DecideAbsence(c, "", "", "")
	errs = append(errs, e)
	_, e = s.Catalog(c)
	errs = append(errs, e)
	_, e = s.RequestAccess(c, AccessInput{})
	errs = append(errs, e)
	_, e = s.ListAccess(c, "")
	errs = append(errs, e)
	_, e = s.DecideAccess(c, "", "", "")
	errs = append(errs, e)
	_, e = s.WhoAmI(c)
	errs = append(errs, e)
	_, e = s.TeamMembers(c)
	errs = append(errs, e)
	for i, e := range errs {
		if toolCode(e) != "outage" {
			t.Errorf("%d: %v", i, e)
		}
	}
}

func TestHTTPAndMCP(t *testing.T) {
	s := newSvc(t)
	signer := identity.NewSigner("k", fixedNow)
	ts := httptest.NewServer(s.Handler(signer, "demo", "adm"))
	defer ts.Close()

	resp, _ := http.Post(ts.URL+"/sso/token", "application/json", bytes.NewBufferString(`{"login":"anna","password":"demo"}`))
	var tok struct{ Token string }
	_ = json.NewDecoder(resp.Body).Decode(&tok)
	resp.Body.Close()
	if tok.Token == "" {
		t.Fatal("sso")
	}
	for body, code := range map[string]int{`{"login":"anna","password":"x"}`: 401, `nope`: 400} {
		r, _ := http.Post(ts.URL+"/sso/token", "application/json", bytes.NewBufferString(body))
		if r.StatusCode != code {
			t.Fatalf("%s: %d", body, r.StatusCode)
		}
	}
	if r, _ := http.Get(ts.URL + "/sso/users"); r.StatusCode != 200 {
		t.Fatal("users")
	}
	if r, _ := http.Get(ts.URL + "/healthz"); r.StatusCode != 200 {
		t.Fatal("health")
	}
	c := mcp.NewClient(ts.URL+"/mcp", time.Second)
	ctx := context.Background()
	tools, err := c.ListTools(ctx, tok.Token)
	if err != nil || len(tools) != 12 { // стажёр не видит *_decide, delegation_set и directory_transfer
		t.Fatalf("tools: %d %v", len(tools), err)
	}
	calls := []struct {
		name string
		args map[string]any
	}{
		{"directory_whoami", nil}, {"directory_team", nil}, {"absence_policy", nil}, {"absence_balance", nil},
		{"absence_create", map[string]any{"date_from": "2026-11-13", "dry_run": true}},
		{"absence_list", map[string]any{"scope": "mine"}}, {"access_catalog", nil},
		{"access_request", map[string]any{"system": "vpn"}}, {"access_list", nil},
		{"access_request", map[string]any{"system": "stash", "project": "search-api", "dry_run": true}},
	}
	for _, cl := range calls {
		if _, err := c.Call(ctx, tok.Token, cl.name, cl.args); err != nil {
			t.Fatalf("%s: %v", cl.name, err)
		}
	}
	if _, err := c.Call(ctx, tok.Token, "absence_get", map[string]any{"id": "x"}); err == nil {
		t.Fatal("absence_get not found")
	}
	if _, err := c.Call(ctx, tok.Token, "absence_create", map[string]any{"date_from": 5}); err == nil {
		t.Fatal("bad args")
	}
	if _, err := c.Call(ctx, tok.Token, "access_request", map[string]any{"system": 5}); err == nil {
		t.Fatal("bad args")
	}
	if _, err := c.Call(ctx, tok.Token, "absence_list", map[string]any{"scope": 5}); err == nil {
		t.Fatal("bad args simple")
	}
	// стажёр не может вызвать решение руководителя даже напрямую
	var ce *mcp.CallError
	if _, err := c.Call(ctx, tok.Token, "absence_decide", map[string]any{"id": "x", "decision": "approved"}); !errors.As(err, &ce) || ce.Code != "forbidden" {
		t.Fatalf("decide by intern: %v", err)
	}
	lead, _ := s.UserByLogin("maria")
	leadTok := signer.Issue(lead, time.Hour)
	for _, n := range []string{"absence_decide", "access_decide"} {
		if _, err := c.Call(ctx, leadTok, n, map[string]any{"id": "x", "decision": "rejected"}); !errors.As(err, &ce) || ce.Code != "not_found" {
			t.Fatalf("%s: %v", n, err)
		}
	}
	// имитация сбоя
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/admin/outage", bytes.NewBufferString(`{"enabled":true}`))
	if r, _ := http.DefaultClient.Do(req); r.StatusCode != 403 {
		t.Fatal("admin without token")
	}
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/admin/outage", bytes.NewBufferString(`{"enabled":true}`))
	req.Header.Set("X-Admin-Token", "adm")
	if r, _ := http.DefaultClient.Do(req); r.StatusCode != 200 {
		t.Fatal("admin")
	}
	var ue *mcp.UnavailableError
	if _, err := c.ListTools(ctx, tok.Token); !errors.As(err, &ue) {
		t.Fatalf("outage: %v", err)
	}
}

func TestWorkdays(t *testing.T) {
	fri := time.Date(2026, 11, 13, 0, 0, 0, 0, msk)
	if got := addWorkdays(fri, 1); got.Weekday() != time.Monday {
		t.Fatal(got)
	}
	if workdaysBetween(fri, fri.AddDate(0, 0, 3)) != 2 {
		t.Fatal("between")
	}
	if humanDate(fri) != "13.11 (пт)" {
		t.Fatal(humanDate(fri))
	}
	if s, err := New(nil, nil); err != nil || s == nil {
		t.Fatal("defaults")
	}
}

func TestDeputies(t *testing.T) {
	s := newSvc(t)
	anna, maria, oleg, elena := claims(s, "anna"), claims(s, "maria"), claims(s, "oleg"), claims(s, "elena")
	r, err := s.CreateAbsence(anna, AbsenceInput{DateFrom: "2026-11-16"})
	if err != nil {
		t.Fatal(err)
	}
	id := r["id"].(string)
	// без делегирования HR BP не видит и не решает заявку руководителя
	if l, _ := s.ListAbsences(elena, "approvals"); len(l["items"].([]Absence)) != 0 {
		t.Fatal("elena sees approvals")
	}
	if _, err := s.DecideAbsence(elena, id, "approved", ""); toolCode(err) != "forbidden" {
		t.Fatal("elena decides")
	}
	// только руководитель назначает заместителя, только из своей команды и не стажёра
	if _, err := s.SetDeputy(oleg, "u-elena", true); toolCode(err) != "forbidden" {
		t.Fatal("mentor sets deputy")
	}
	for id, code := range map[string]string{"u-pavel": "other_team", "u-anna": "not_allowed_for_role", "u-none": "not_found", "u-maria": "not_found"} {
		if _, err := s.SetDeputy(maria, id, true); toolCode(err) != code {
			t.Fatalf("%s: %v", id, err)
		}
	}
	d, err := s.SetDeputy(maria, "u-elena", true)
	if err != nil || len(d["deputies"].([]map[string]any)) != 1 {
		t.Fatalf("deputy: %v %v", d, err)
	}
	if l, _ := s.Deputies(elena); len(l["actingFor"].([]map[string]any)) != 1 {
		t.Fatal("acting for")
	}
	if l, _ := s.ListAbsences(elena, "approvals"); len(l["items"].([]Absence)) != 1 {
		t.Fatal("deputy sees approvals")
	}
	if _, err := s.GetAbsence(elena, id); err != nil {
		t.Fatal("deputy reads absence")
	}
	res, err := s.DecideAbsence(elena, id, "approved", "за Марию")
	if err != nil || res["decidedBy"] != "u-elena" {
		t.Fatalf("deputy decides: %v %v", res, err)
	}
	// доступ: согласующий — руководитель, решает заместитель
	acc, err := s.RequestAccess(anna, AccessInput{System: "redash", Justification: "Отчёты по поиску для задачи"})
	if err != nil || acc["approver_id"] != "u-maria" {
		t.Fatalf("access: %v %v", acc, err)
	}
	if l, _ := s.ListAccess(elena, "approvals"); len(l["items"].([]AccessRequest)) != 1 {
		t.Fatal("deputy sees access approvals")
	}
	if res, err := s.DecideAccess(elena, acc["id"].(string), "granted", ""); err != nil || res["decidedBy"] != "u-elena" {
		t.Fatalf("deputy access: %v %v", res, err)
	}
	if d, _ := s.SetDeputy(maria, "u-elena", false); len(d["deputies"].([]map[string]any)) != 0 {
		t.Fatal("revoke")
	}
	if _, err := s.DecideAbsence(elena, id, "approved", ""); toolCode(err) != "forbidden" {
		t.Fatal("revoked deputy decides")
	}
	s.SetOutage(true)
	if _, err := s.SetDeputy(maria, "u-elena", true); err == nil {
		t.Fatal("outage set")
	}
	if _, err := s.Deputies(maria); err == nil {
		t.Fatal("outage list")
	}
}

func TestRegisterAndTransfer(t *testing.T) {
	s := newSvc(t)
	if ts := s.Teams(); len(ts) != 2 || ts[0].LeadName == "" {
		t.Fatalf("teams with lead: %+v", ts)
	}
	ok := RegisterInput{Name: "  Пётр   Сидоров ", Email: "Petr@Example.com", Password: "secret123", Team: "search", Role: "intern"}
	for name, bad := range map[string]RegisterInput{
		"name":     {Name: "Пётр", Email: ok.Email, Password: ok.Password, Team: "search", Role: "intern"},
		"email":    {Name: ok.Name, Email: "petr", Password: ok.Password, Team: "search", Role: "intern"},
		"email2":   {Name: ok.Name, Email: "a b@x.ru", Password: ok.Password, Team: "search", Role: "intern"},
		"password": {Name: ok.Name, Email: ok.Email, Password: "short1", Team: "search", Role: "intern"},
		"digits":   {Name: ok.Name, Email: ok.Email, Password: "onlyletters", Team: "search", Role: "intern"},
		"role":     {Name: ok.Name, Email: ok.Email, Password: ok.Password, Team: "search", Role: "lead"},
		"team":     {Name: ok.Name, Email: ok.Email, Password: ok.Password, Team: "platform", Role: "intern"},
	} {
		if _, err := s.Register(bad); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	u, err := s.Register(ok)
	if err != nil || u.Name != "Пётр Сидоров" || u.Login != "petr@example.com" || u.LeadID != "u-maria" || u.MentorID != "u-oleg" || !u.Registered {
		t.Fatalf("register: %+v %v", u, err)
	}
	if _, err := s.Register(ok); err == nil {
		t.Fatal("duplicate email")
	}
	if _, ok := s.CheckPassword("PETR@example.com", "secret123", "buddy2026"); !ok {
		t.Fatal("login by email")
	}
	if _, ok := s.CheckPassword("petr@example.com", "buddy2026", "buddy2026"); ok {
		t.Fatal("demo password for registered user")
	}
	if _, ok := s.CheckPassword("anna", "buddy2026", "buddy2026"); !ok {
		t.Fatal("demo login")
	}
	if _, ok := s.CheckPassword("none", "x", "buddy2026"); ok {
		t.Fatal("unknown login")
	}
	eng, _ := s.Register(RegisterInput{Name: "Инна Ли", Email: "inna@example.com", Password: "secret123", Team: "delivery", Role: "engineer"})
	if eng.LeadID != "u-sergey" || eng.MentorID != "" {
		t.Fatalf("engineer: %+v", eng)
	}
	if n := len(s.Users()); n != 10 {
		t.Fatalf("users incl. registered: %d", n)
	}
	// перевод
	maria, oleg := claims(s, "maria"), claims(s, "oleg")
	r, err := s.CreateAbsence(claims(s, "anna"), AbsenceInput{DateFrom: "2026-11-16"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.SetDeputy(maria, "u-oleg", true)
	for name, c := range map[string]struct {
		cl         identity.Claims
		user, team string
		code       string
	}{
		"mentor":     {oleg, "u-anna", "delivery", "forbidden"},
		"self":       {maria, "u-maria", "delivery", "self"},
		"other team": {maria, "u-ivan", "search", "other_team"},
		"same":       {maria, "u-anna", "search", "same_team"},
		"bad team":   {maria, "u-anna", "platform", "bad_team"},
		"unknown":    {maria, "u-none", "delivery", "not_found"},
	} {
		if _, err := s.Transfer(c.cl, c.user, c.team); toolCode(err) != c.code {
			t.Errorf("%s: %v", name, err)
		}
	}
	res, err := s.Transfer(maria, "u-anna", "delivery")
	if err != nil || res["movedRequests"] != 1 {
		t.Fatalf("transfer: %v %v", res, err)
	}
	anna, _ := s.UserByLogin("anna")
	if anna.Team != "delivery" || anna.LeadID != "u-sergey" || anna.MentorID != "u-dmitry" || anna.TeamName == "" {
		t.Fatalf("anna after transfer: %+v", anna)
	}
	if a, _ := s.GetAbsence(claims(s, "sergey"), r["id"].(string)); a["approverId"] != "u-sergey" {
		t.Fatal("pending absence moved to new lead")
	}
	// старый токен Анны: справочник обновляет отдел
	if c := s.refresh(claims(s, "anna")); c.Team != "delivery" {
		t.Fatal("refresh claims")
	}
	// заместитель переведён — больше не заместитель
	if _, err := s.Transfer(maria, "u-oleg", "delivery"); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.Deputies(maria); len(d["deputies"].([]map[string]any)) != 0 {
		t.Fatal("deputy kept after transfer")
	}
	if c := s.refresh(identity.Claims{Sub: "u-none"}); c.Sub != "u-none" {
		t.Fatal("refresh unknown")
	}
	s.SetOutage(true)
	if _, err := s.Transfer(maria, "u-elena", "delivery"); err == nil {
		t.Fatal("outage transfer")
	}
	if _, err := s.Register(RegisterInput{Name: "Аа Бб", Email: "x@example.com", Password: "secret123", Team: "search", Role: "intern"}); err == nil {
		t.Fatal("outage register")
	}
}

func TestStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "corp.json")
	s := newSvc(t)
	if err := s.UseStateFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register(RegisterInput{Name: "Пётр Сидоров", Email: "petr@example.com", Password: "secret123", Team: "search", Role: "intern"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Transfer(claims(s, "maria"), "u-elena", "delivery"); err != nil {
		t.Fatal(err)
	}
	s2 := newSvc(t)
	if err := s2.UseStateFile(path); err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.CheckPassword("petr@example.com", "secret123", ""); !ok {
		t.Fatal("registered user restored")
	}
	if e, _ := s2.UserByLogin("elena"); e.Team != "delivery" {
		t.Fatal("transfer restored")
	}
	u, _ := s2.Register(RegisterInput{Name: "Инна Ли", Email: "inna@example.com", Password: "secret123", Team: "search", Role: "engineer"})
	if u.ID == "" || u.ID == "u-1001" {
		t.Fatalf("id sequence continues: %s", u.ID)
	}
	_ = os.WriteFile(path, []byte("{bad"), 0o600)
	if err := newSvc(t).UseStateFile(path); err == nil {
		t.Fatal("bad state")
	}
	if err := newSvc(t).UseStateFile(t.TempDir()); err == nil {
		t.Fatal("dir as state")
	}
}
