package api

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"aibuddy/internal/app"
	"aibuddy/internal/domain"
	"aibuddy/internal/onboarding"
	"aibuddy/internal/quiz"
)

type meResp struct {
	User  domain.User
	Perms []string
}

func perms(c *client) []string {
	var m meResp
	c.do("GET", "/api/me", nil, &m)
	return m.Perms
}

type kbResp struct {
	Docs    []docView
	Folders []string
}

func TestRightsAndKBEditing(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	maria, oleg, elena, anna, ivan := e.login("maria"), e.login("oleg"), e.login("elena"), e.login("anna"), e.login("ivan")
	if p := perms(maria); !slices.Contains(p, "kb") || !slices.Contains(p, "approvals") || !slices.Contains(p, "programs") {
		t.Fatalf("lead perms: %v", p)
	}
	if p := perms(oleg); slices.Contains(p, "kb") || !slices.Contains(p, "programs") {
		t.Fatalf("mentor perms: %v", p)
	}
	if p := perms(anna); len(p) != 0 {
		t.Fatalf("intern perms: %v", p)
	}
	// права видит и меняет только руководитель
	if code := oleg.do("GET", "/api/rights", nil, nil); code != 403 {
		t.Fatal("mentor rights")
	}
	var rights struct{ Members []app.Member }
	if code := maria.do("GET", "/api/rights", nil, &rights); code != 200 || len(rights.Members) != 2 {
		t.Fatalf("rights: %d %+v", code, rights)
	}
	for body, code := range map[string]int{
		`{"userId":"u-anna","perm":"kb","enabled":true}`:  400, // стажёру нельзя
		`{"userId":"u-pavel","perm":"kb","enabled":true}`: 404, // другая команда
		`{"userId":"u-elena","perm":"x","enabled":true}`:  400,
	} {
		var in map[string]any
		_ = json.Unmarshal([]byte(body), &in)
		if got := maria.do("POST", "/api/rights", in, nil); got != code {
			t.Fatalf("%s: %d", body, got)
		}
	}
	if code := oleg.do("POST", "/api/rights", map[string]any{"userId": "u-elena", "perm": "kb", "enabled": true}, nil); code != 403 {
		t.Fatal("mentor grants")
	}
	// без права HR BP не пишет инструкции
	doc := map[string]any{"title": "Как проходит релиз поиска", "folder": " Процессы / Релизы ", "audience": []string{"intern", "mentor"},
		"tags": []string{"Релиз", "релиз", "деплой"}, "body": "Релизы поиска — по вторникам.\n\n## Чек-лист\n1. Зелёный CI.\n2. Ревью наставника."}
	if code := elena.do("POST", "/api/kb", doc, nil); code != 403 {
		t.Fatal("hrbp without grant")
	}
	if code := maria.do("POST", "/api/rights", map[string]any{"userId": "u-elena", "perm": "kb", "enabled": true}, &rights); code != 200 {
		t.Fatal("grant kb")
	}
	if !slices.Contains(perms(elena), "kb") {
		t.Fatal("elena kb perm")
	}
	var notes struct{ Notifications []domain.Notification }
	elena.do("GET", "/api/notifications", nil, &notes)
	if len(notes.Notifications) == 0 || !strings.Contains(notes.Notifications[0].Text, "инструкции") {
		t.Fatalf("grant notification: %+v", notes)
	}
	var created docView
	if code := elena.do("POST", "/api/kb", doc, &created); code != 201 || created.Folder != "Процессы/Релизы" || !created.Editable || len(created.Sections) != 2 {
		t.Fatalf("create doc: %d %+v", code, created)
	}
	// стажёр команды видит инструкцию в папке и находит поиском; другая команда — нет
	var list kbResp
	anna.do("GET", "/api/kb", nil, &list)
	if !slices.Contains(list.Folders, "Процессы") || !slices.Contains(list.Folders, "Процессы/Релизы") || !slices.Contains(list.Folders, "Инструменты") {
		t.Fatalf("folders: %v", list.Folders)
	}
	found := false
	for _, d := range list.Docs {
		if d.ID == created.ID {
			found = !d.Editable
		}
	}
	if !found {
		t.Fatal("anna sees doc, not editable")
	}
	anna.do("GET", "/api/kb?q=релиз", nil, &list)
	if !slices.ContainsFunc(list.Docs, func(d docView) bool { return d.ID == created.ID }) {
		t.Fatalf("search: %+v", list.Docs)
	}
	anna.do("GET", "/api/kb?q=ssh", nil, &list)
	if len(list.Docs) == 0 || list.Docs[0].ID != "tool-stash-keys" {
		t.Fatalf("search content: %+v", list.Docs)
	}
	if code := ivan.do("GET", "/api/kb/"+created.ID, nil, nil); code != 404 {
		t.Fatal("other team reads doc")
	}
	// агент отвечает по новой инструкции
	if m, _ := anna.ask("когда проходят релизы поиска?"); !strings.Contains(m.Text, "вторник") {
		t.Fatalf("agent uses new doc: %s", m.Text)
	}
	// правка, ошибки валидации, встроенные инструкции
	doc["body"] = "Релизы поиска — по средам.\n\n## Чек-лист\n1. Зелёный CI."
	if code := elena.do("PUT", "/api/kb/"+created.ID, doc, &created); code != 200 || !strings.Contains(created.Body, "средам") {
		t.Fatal("edit doc")
	}
	for _, bad := range []map[string]any{{"title": "x", "body": "достаточно длинный текст"}, {"title": "Норм", "body": "коротко"},
		{"title": "Норм", "body": "достаточно длинный текст", "audience": []string{"boss"}}, {"title": "Норм", "body": "достаточно длинный текст", "folder": "a/b/c/d/e"}} {
		if code := elena.do("POST", "/api/kb", bad, nil); code != 400 {
			t.Fatalf("validation %v: %d", bad, code)
		}
	}
	if code := elena.do("PUT", "/api/kb/tool-stash-keys", doc, nil); code != 400 {
		t.Fatal("builtin edit")
	}
	if code := elena.do("DELETE", "/api/kb/tool-stash-keys", nil, nil); code != 400 {
		t.Fatal("builtin delete")
	}
	if code := elena.do("PUT", "/api/kb/none", doc, nil); code != 404 {
		t.Fatal("edit missing")
	}
	if code := anna.do("DELETE", "/api/kb/"+created.ID, nil, nil); code != 403 {
		t.Fatal("intern deletes")
	}
	// папки: пустую можно удалить, с инструкциями — нет
	var folders struct{ Folders []string }
	if code := elena.do("POST", "/api/kb/folders", map[string]string{"path": "Онбординг/Вебинары"}, &folders); code != 201 || !slices.Contains(folders.Folders, "Онбординг/Вебинары") {
		t.Fatalf("folder: %d %v", code, folders)
	}
	if code := elena.do("POST", "/api/kb/folders", map[string]string{"path": " / "}, nil); code != 400 {
		t.Fatal("empty folder")
	}
	if code := anna.do("POST", "/api/kb/folders", map[string]string{"path": "X"}, nil); code != 403 {
		t.Fatal("intern folder")
	}
	if code := elena.do("DELETE", "/api/kb/folders?path=Процессы", nil, nil); code != 400 {
		t.Fatal("non-empty folder")
	}
	if code := elena.do("DELETE", "/api/kb/folders?path=Онбординг", nil, &folders); code != 200 || slices.Contains(folders.Folders, "Онбординг/Вебинары") {
		t.Fatalf("delete folder: %v", folders)
	}
	if code := anna.do("DELETE", "/api/kb/folders?path=Онбординг", nil, nil); code != 403 {
		t.Fatal("intern deletes folder")
	}
	if code := elena.do("DELETE", "/api/kb/"+created.ID, nil, nil); code != 200 {
		t.Fatal("delete doc")
	}
	if code := elena.do("DELETE", "/api/kb/"+created.ID, nil, nil); code != 404 {
		t.Fatal("delete twice")
	}
	// забрать право
	maria.do("POST", "/api/rights", map[string]any{"userId": "u-elena", "perm": "kb", "enabled": false}, &rights)
	if slices.Contains(perms(elena), "kb") {
		t.Fatal("revoke kb")
	}
}

func TestApprovalDelegation(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	maria, elena, anna := e.login("maria"), e.login("elena"), e.login("anna")
	if slices.Contains(perms(elena), "approvals") {
		t.Fatal("elena approvals before")
	}
	var rights struct{ Members []app.Member }
	if code := maria.do("POST", "/api/rights", map[string]any{"userId": "u-elena", "perm": "approvals", "enabled": true}, &rights); code != 200 {
		t.Fatal("delegate")
	}
	for _, m := range rights.Members {
		if m.User.ID == "u-elena" && !slices.Contains(m.Perms, "approvals") {
			t.Fatalf("rights view: %+v", m)
		}
	}
	if !slices.Contains(perms(elena), "approvals") {
		t.Fatal("elena approvals perm")
	}
	_, act := anna.ask("хочу отгул 16 ноября")
	if act == nil {
		t.Fatal("absence")
	}
	anna.do("POST", "/api/actions/"+act.ID+"/confirm", nil, nil)
	var ap struct{ Approvals []app.Approval }
	elena.do("GET", "/api/approvals", nil, &ap)
	if len(ap.Approvals) != 1 {
		t.Fatalf("deputy approvals: %+v", ap)
	}
	if code := elena.do("POST", "/api/approvals/absence/"+ap.Approvals[0].ExternalID, map[string]any{"approve": true, "comment": "За Марию"}, nil); code != 200 {
		t.Fatal("deputy decides")
	}
	var acts struct{ Actions []domain.Action }
	anna.do("GET", "/api/actions", nil, &acts)
	if acts.Actions[0].Status != domain.ActionApproved {
		t.Fatalf("approved: %+v", acts.Actions[0])
	}
	maria.do("POST", "/api/rights", map[string]any{"userId": "u-elena", "perm": "approvals", "enabled": false}, nil)
	if slices.Contains(perms(elena), "approvals") {
		t.Fatal("revoke approvals")
	}
	e.corp.SetOutage(true)
	if code := maria.do("POST", "/api/rights", map[string]any{"userId": "u-elena", "perm": "approvals", "enabled": true}, nil); code != 503 {
		t.Fatalf("outage: %d", code)
	}
	if code := maria.do("GET", "/api/rights", nil, nil); code != 503 {
		t.Fatal("outage rights")
	}
}

func TestQuizzes(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna, oleg, dmitry, maria := e.login("anna"), e.login("oleg"), e.login("dmitry"), e.login("maria")
	var list struct{ Quizzes []app.QuizInfo }
	anna.do("GET", "/api/quizzes", nil, &list)
	if len(list.Quizzes) < 4 {
		t.Fatalf("quizzes: %+v", list)
	}
	// прохождение: ответы не уходят в интерфейс
	var raw map[string]any
	anna.do("GET", "/api/quizzes/quiz-week1", nil, &raw)
	b, _ := json.Marshal(raw)
	if strings.Contains(string(b), "correct") || strings.Contains(string(b), "explain") {
		t.Fatalf("answers leaked: %s", b)
	}
	if code := anna.do("GET", "/api/quizzes/quiz-week1/edit", nil, nil); code != 403 {
		t.Fatal("intern edit view")
	}
	// шаг с тестом нельзя закрыть вручную
	if code := anna.do("PATCH", "/api/plan/steps/w1-quiz", map[string]string{"status": "done"}, nil); code != 400 {
		t.Fatal("manual quiz step")
	}
	if code := anna.do("POST", "/api/quizzes/quiz-week1/submit", map[string]any{"answers": []int{1}}, nil); code != 400 {
		t.Fatal("partial answers")
	}
	var res app.QuizResult
	anna.do("POST", "/api/quizzes/quiz-week1/submit", map[string]any{"answers": []int{0, 0, 0, 0, 0}}, &res)
	if res.Attempt.Passed || res.Attempt.Correct != 2 || len(res.Review) != 5 || res.Review[0].Explain == "" {
		t.Fatalf("failed attempt: %+v", res.Attempt)
	}
	anna.do("POST", "/api/quizzes/quiz-week1/submit", map[string]any{"answers": []int{1, 0, 1, 2, 0}}, &res)
	if !res.Attempt.Passed || res.Attempt.Percent != 100 || !slices.Contains(res.StepsDone, "w1-quiz") {
		t.Fatalf("passed attempt: %+v", res)
	}
	var plan onboarding.Plan
	anna.do("GET", "/api/plan", nil, &plan)
	for _, per := range plan.Periods {
		for _, s := range per.Steps {
			if s.ID == "w1-quiz" && s.Status != domain.StepDone {
				t.Fatal("quiz step not done")
			}
		}
	}
	anna.do("GET", "/api/quizzes", nil, &list)
	for _, q := range list.Quizzes {
		if q.ID == "quiz-week1" && (!q.Passed || q.Attempts != 2 || q.Best != 100) {
			t.Fatalf("summary: %+v", q)
		}
	}
	// наставник получил уведомление и видит результаты; чужой наставник — нет
	var notes struct{ Notifications []domain.Notification }
	oleg.do("GET", "/api/notifications", nil, &notes)
	if len(notes.Notifications) < 2 || !strings.Contains(notes.Notifications[0].Text, "сдал") {
		t.Fatalf("mentor notified: %+v", notes)
	}
	var atts struct{ Attempts []quiz.Attempt }
	if code := oleg.do("GET", "/api/quiz-results?internId=u-anna", nil, &atts); code != 200 || len(atts.Attempts) != 2 {
		t.Fatal("mentor results")
	}
	if code := dmitry.do("GET", "/api/quiz-results?internId=u-anna", nil, nil); code != 403 {
		t.Fatal("other mentor results")
	}
	if code := anna.do("GET", "/api/quiz-results", nil, &atts); code != 200 || len(atts.Attempts) != 2 {
		t.Fatal("own results")
	}
	var team struct{ Interns []app.InternSummary }
	oleg.do("GET", "/api/team", nil, &team)
	if team.Interns[0].QuizzesPassed != 1 || team.Interns[0].TemplateName == "" {
		t.Fatalf("team quiz summary: %+v", team.Interns[0])
	}
	// руководитель создаёт тест по инструкции
	q := map[string]any{"title": "Ключи и Stash", "docIds": []string{"tool-stash-keys"}, "passPercent": 50,
		"questions": []map[string]any{{"text": "Какую часть ключа добавить?", "options": []string{"Публичную", "Приватную"}, "correct": 0}}}
	if code := anna.do("POST", "/api/quizzes", q, nil); code != 403 {
		t.Fatal("intern creates quiz")
	}
	var saved quiz.Quiz
	if code := maria.do("POST", "/api/quizzes", q, &saved); code != 201 || saved.Team != "search" {
		t.Fatalf("create quiz: %d %+v", code, saved)
	}
	for _, bad := range []map[string]any{
		{"title": "Тест", "questions": []map[string]any{}},
		{"title": "Тест", "questions": []map[string]any{{"text": "?", "options": []string{"a"}, "correct": 0}}},
		{"title": "Тест", "questions": []map[string]any{{"text": "?", "options": []string{"a", "b"}, "correct": 5}}},
		{"title": "Тест", "docIds": []string{"none"}, "questions": []map[string]any{{"text": "?", "options": []string{"a", "b"}, "correct": 0}}},
	} {
		if code := maria.do("POST", "/api/quizzes", bad, nil); code != 400 {
			t.Fatalf("bad quiz %v: %d", bad, code)
		}
	}
	q["title"] = "Ключи, Stash и доступы"
	if code := maria.do("PUT", "/api/quizzes/"+saved.ID, q, &saved); code != 200 || saved.Title != "Ключи, Stash и доступы" {
		t.Fatal("edit quiz")
	}
	if code := maria.do("PUT", "/api/quizzes/quiz-week1", q, nil); code != 400 {
		t.Fatal("edit builtin quiz")
	}
	var builtin quiz.Quiz
	if code := maria.do("GET", "/api/quizzes/quiz-week1/edit", nil, &builtin); code != 200 || len(builtin.Questions) != 5 {
		t.Fatal("editor sees builtin answers")
	}
	if code := dmitry.do("GET", "/api/quizzes/"+saved.ID, nil, nil); code != 404 {
		t.Fatal("other team quiz")
	}
	if code := maria.do("DELETE", "/api/quizzes/quiz-week1", nil, nil); code != 400 {
		t.Fatal("delete builtin quiz")
	}
	if code := maria.do("DELETE", "/api/quizzes/none", nil, nil); code != 404 {
		t.Fatal("delete missing quiz")
	}
}

func TestTemplatesAndPrograms(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna, oleg, dmitry, maria := e.login("anna"), e.login("oleg"), e.login("dmitry"), e.login("maria")
	if code := anna.do("GET", "/api/templates", nil, nil); code != 403 {
		t.Fatal("intern templates")
	}
	var tl struct{ Templates []onboarding.Template }
	oleg.do("GET", "/api/templates", nil, &tl)
	if len(tl.Templates) != 2 || tl.Templates[0].ID != "intern-dev" || !tl.Templates[0].Builtin {
		t.Fatalf("templates: %+v", tl.Templates)
	}
	analyst := tl.Templates[1]
	// встроенный шаблон не меняется — делаем копию с новым шагом
	if code := oleg.do("PUT", "/api/templates/intern-dev", analyst, nil); code != 400 {
		t.Fatal("edit builtin template")
	}
	cp := analyst
	cp.ID, cp.Name = "", "Аналитик поиска"
	cp.Steps = append(append([]onboarding.Step{}, analyst.Steps...), onboarding.Step{Period: "week2", Title: "Разбери дашборд качества поиска", Description: "С наставником"})
	var saved onboarding.Template
	if code := oleg.do("POST", "/api/templates", cp, &saved); code != 201 || saved.Builtin || saved.Team != "search" || saved.Steps[len(saved.Steps)-1].ID == "" {
		t.Fatalf("create template: %d %+v", code, saved.ID)
	}
	bad := saved
	bad.Steps = append(append([]onboarding.Step{}, saved.Steps...), onboarding.Step{Period: "nope", Title: "Шаг без этапа"})
	if code := oleg.do("PUT", "/api/templates/"+saved.ID, bad, nil); code != 400 {
		t.Fatal("bad period")
	}
	bad.Steps = []onboarding.Step{{Period: "week1", Title: "Тест", Quiz: "none"}}
	if code := oleg.do("PUT", "/api/templates/"+saved.ID, bad, nil); code != 400 {
		t.Fatal("unknown quiz")
	}
	if code := anna.do("POST", "/api/templates", cp, nil); code != 403 {
		t.Fatal("intern creates template")
	}
	dmitry.do("GET", "/api/templates", nil, &tl)
	if len(tl.Templates) != 2 {
		t.Fatal("other team sees template")
	}
	// назначение шаблона стажёру
	if code := dmitry.do("POST", "/api/team/u-anna/program/assign", map[string]string{"templateId": "intern-analyst"}, nil); code != 403 {
		t.Fatal("other mentor assigns")
	}
	if code := oleg.do("POST", "/api/team/u-anna/program/assign", map[string]string{"templateId": "none"}, nil); code != 404 {
		t.Fatal("unknown template")
	}
	var prog app.Program
	if code := oleg.do("POST", "/api/team/u-anna/program/assign", map[string]string{"templateId": saved.ID}, &prog); code != 200 || prog.BaseID != saved.ID || prog.Customized {
		t.Fatalf("assign: %d %+v", code, prog.BaseID)
	}
	var plan onboarding.Plan
	anna.do("GET", "/api/plan", nil, &plan)
	if plan.TemplateName != "Аналитик поиска" || plan.Total != len(saved.Steps) {
		t.Fatalf("plan after assign: %s %d", plan.TemplateName, plan.Total)
	}
	// настройка под стажёра: убрать шаг и добавить свой
	if code := maria.do("GET", "/api/team/u-anna/program", nil, &prog); code != 200 {
		t.Fatal("lead reads program")
	}
	steps := append([]onboarding.Step{}, prog.Template.Steps[1:]...)
	steps = append(steps, onboarding.Step{Period: "month1", Owner: "intern", Title: "Подготовь доклад для команды", Category: "Команда"})
	if code := maria.do("PUT", "/api/team/u-anna/program", map[string]any{"steps": steps}, &prog); code != 200 || !prog.Customized {
		t.Fatal("customize")
	}
	anna.do("GET", "/api/plan", nil, &plan)
	if !plan.Customized || plan.Total != len(steps) {
		t.Fatalf("customized plan: %v %d", plan.Customized, plan.Total)
	}
	if code := maria.do("PUT", "/api/team/u-anna/program", map[string]any{"steps": []any{}}, nil); code != 400 {
		t.Fatal("empty program")
	}
	if code := anna.do("PUT", "/api/team/u-anna/program", map[string]any{"steps": steps}, nil); code != 403 {
		t.Fatal("intern edits program")
	}
	if code := dmitry.do("GET", "/api/team/u-anna/program", nil, nil); code != 403 {
		t.Fatal("other mentor reads program")
	}
	if code := oleg.do("GET", "/api/team/u-oleg/program", nil, nil); code != 404 {
		t.Fatal("program of non-intern")
	}
	// отметка шага собственной программы
	if code := anna.do("PATCH", "/api/plan/steps/"+prog.Template.Steps[len(prog.Template.Steps)-1].ID, map[string]string{"status": "done"}, nil); code != 200 {
		t.Fatal("custom step done")
	}
	// удаление шаблона не ломает программу стажёра
	if code := oleg.do("DELETE", "/api/templates/intern-dev", nil, nil); code != 400 {
		t.Fatal("delete builtin")
	}
	if code := oleg.do("DELETE", "/api/templates/"+saved.ID, nil, nil); code != 200 {
		t.Fatal("delete template")
	}
	if code := anna.do("GET", "/api/plan", nil, &plan); code != 200 || plan.TemplateName != "Аналитик поиска" {
		t.Fatal("program survives template deletion")
	}
	var notes struct{ Notifications []domain.Notification }
	anna.do("GET", "/api/notifications", nil, &notes)
	if len(notes.Notifications) < 2 {
		t.Fatal("intern notified about program")
	}
}

func TestChatMarksStepsAndOffersQuiz(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna := e.login("anna")
	m, _ := anna.ask("Отметь выполненным: Настрой avito-cli")
	if !strings.Contains(m.Text, "Отметил шаг **Настрой avito-cli**") {
		t.Fatalf("step done: %s", m.Text)
	}
	var plan onboarding.Plan
	anna.do("GET", "/api/plan", nil, &plan)
	done := false
	for _, per := range plan.Periods {
		for _, s := range per.Steps {
			if s.ID == "w1-avito-cli" {
				done = s.Status == domain.StepDone
			}
		}
	}
	if !done {
		t.Fatal("plan not updated")
	}
	if m, _ := anna.ask("я сделал шаг avito-cli"); !strings.Contains(m.Text, "уже отмечен") {
		t.Fatalf("already done: %s", m.Text)
	}
	if m, _ := anna.ask("отметь шаг про регулярные 1:1"); !strings.Contains(m.Text, "отмечает наставник") {
		t.Fatalf("mentor step: %s", m.Text)
	}
	if m, _ := anna.ask("отметь шаг тест доступы и инструменты"); m.QuizID != "quiz-week1" {
		t.Fatalf("quiz step: %s %s", m.Text, m.QuizID)
	}
	if m, _ := anna.ask("отметь что-нибудь"); len(m.Suggestions) == 0 || !strings.HasPrefix(m.Suggestions[0], "Отметь выполненным: ") {
		t.Fatalf("unknown step: %+v", m)
	}
	if m, _ := anna.ask("Я изучил принципы разработки"); !strings.Contains(m.Text, "Отметил") {
		t.Fatalf("natural phrase: %s", m.Text)
	}
	m, _ = anna.ask("проверь мои знания по SSH-ключам и Stash")
	if m.QuizID != "quiz-week1" || !strings.Contains(m.Text, "Доступы и инструменты") {
		t.Fatalf("quiz by topic: %s %s", m.Text, m.QuizID)
	}
	if m, _ := anna.ask("хочу пройти тест"); m.QuizID == "" {
		t.Fatal("quiz intent")
	}
	// «тестовый стенд» — не тест
	if m, _ := anna.ask("что такое тестовый стенд?"); m.QuizID != "" {
		t.Fatal("false quiz intent")
	}
	// наставник не отмечает шаги через чат
	if m, _ := e.login("oleg").ask("отметь выполненным avito-cli"); !strings.Contains(m.Text, "только в своём плане") {
		t.Fatalf("mentor chat step: %s", m.Text)
	}
}
