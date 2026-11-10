package corp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
	if len(s.Users()) != 7 {
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
	if len(items) != 10 {
		t.Fatalf("catalog: %d", len(items))
	}
	for _, it := range items {
		if it["id"] == "k8s-prod" && it["available"] != false {
			t.Fatal("k8s must be unavailable for intern")
		}
	}
	cases := map[string]AccessInput{
		"unknown_system":         {System: "nope"},
		"not_allowed_for_role":   {System: "k8s-prod"},
		"bad_level":              {System: "stash", Level: "admin"},
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
	r, err := s.RequestAccess(anna, AccessInput{System: "grafana"})
	if err != nil || r["status"] != "granted" {
		t.Fatalf("auto: %v %v", r, err)
	}
	if _, err := s.RequestAccess(anna, AccessInput{System: "grafana"}); toolCode(err) != "duplicate" {
		t.Fatal("dup granted")
	}
	dry, err := s.RequestAccess(anna, AccessInput{System: "stash", Level: "write", DryRun: true})
	if err != nil || dry["status"] != "valid" || dry["approver"] != "Мария Волкова" {
		t.Fatalf("dry: %v %v", dry, err)
	}
	r, err = s.RequestAccess(anna, AccessInput{System: "stash", Level: "write"})
	if err != nil || r["status"] != "pending" {
		t.Fatalf("pending: %v %v", r, err)
	}
	id := r["id"].(string)
	if _, err := s.RequestAccess(anna, AccessInput{System: "stash"}); toolCode(err) != "duplicate" {
		t.Fatal("dup pending")
	}
	cat, _ = s.Catalog(anna)
	for _, it := range cat["items"].([]map[string]any) {
		if it["id"] == "stash" && it["myStatus"] != "pending" {
			t.Fatal("my status")
		}
	}
	maria := claims(s, "maria")
	if l, _ := s.ListAccess(maria, "approvals"); len(l["items"].([]AccessRequest)) != 1 {
		t.Fatal("approvals")
	}
	if _, err := s.DecideAccess(claims(s, "sergey"), id, "granted", ""); toolCode(err) != "forbidden" {
		t.Fatal("foreign lead")
	}
	if _, err := s.DecideAccess(maria, id, "ok", ""); toolCode(err) != "bad_decision" {
		t.Fatal("bad decision")
	}
	if _, err := s.DecideAccess(maria, "none", "granted", ""); toolCode(err) != "not_found" {
		t.Fatal("not found")
	}
	if d, err := s.DecideAccess(maria, id, "granted", "ok"); err != nil || d["status"] != "granted" {
		t.Fatalf("decide: %v %v", d, err)
	}
	if _, err := s.DecideAccess(maria, id, "granted", ""); toolCode(err) != "already_decided" {
		t.Fatal("already")
	}
	if l, _ := s.ListAccess(anna, "mine"); len(l["items"].([]AccessRequest)) != 2 {
		t.Fatal("mine")
	}
	if _, err := s.RequestAccess(anna, AccessInput{System: "redash", Justification: "Посчитать метрики поиска для задачи SEARCH-101"}); err != nil {
		t.Fatal(err)
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
	if err != nil || len(tools) != 10 { // intern не видит *_decide
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
