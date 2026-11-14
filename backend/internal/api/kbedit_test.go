package api

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"aibuddy/internal/domain"
	"aibuddy/internal/llm"
)

type kbList struct {
	Docs    []docView
	Folders []string
}

func TestEditDeleteBuiltinDocs(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	maria, anna, admin := e.login("maria"), e.login("anna"), e.login("admin")
	var d docView
	if code := maria.do("GET", "/api/kb/tool-stash-keys", nil, &d); code != 200 || !d.Editable || !strings.Contains(d.Body, "##") {
		t.Fatalf("builtin editable for lead: %d %+v", code, d.Editable)
	}
	if anna.do("GET", "/api/kb/tool-stash-keys", nil, &d); d.Editable {
		t.Fatal("intern can edit")
	}
	in := map[string]any{"title": "SSH-ключи в Stash (обновлено)", "folder": "Инструменты", "audience": []string{"all"}, "body": "## Ключ\n1. Сгенерируй ed25519."}
	if code := maria.do("PUT", "/api/kb/tool-stash-keys", in, &d); code != 200 || d.Title != "SSH-ключи в Stash (обновлено)" {
		t.Fatalf("edit builtin: %d", code)
	}
	// после перезапуска правка остаётся
	if err := e.app.LoadKB(context.Background()); err != nil {
		t.Fatal(err)
	}
	anna.do("GET", "/api/kb/tool-stash-keys", nil, &d)
	if !strings.Contains(d.Title, "обновлено") {
		t.Fatal("edit lost")
	}
	// команда другого отдела не правит инструкции чужого отдела
	if code := e.login("sergey").do("DELETE", "/api/kb/team-search", nil, nil); code != 403 {
		t.Fatalf("foreign team doc: %d", code)
	}
	for c, id := range map[*client]string{maria: "tool-stash-keys", admin: "team-delivery"} {
		if code := c.do("DELETE", "/api/kb/"+id, nil, nil); code != 200 {
			t.Fatalf("delete %s: %d", id, code)
		}
	}
	// удалённая встроенная инструкция не возвращается при запуске
	if err := e.app.LoadKB(context.Background()); err != nil {
		t.Fatal(err)
	}
	if code := anna.do("GET", "/api/kb/tool-stash-keys", nil, nil); code != 404 {
		t.Fatalf("deleted builtin back: %d", code)
	}
}

func answerQuestion(t *testing.T, e *testEnv, text, answer string) {
	t.Helper()
	q := domain.Question{ID: "q-" + string([]rune(strings.ReplaceAll(text, " ", ""))[:5]), InternID: "u-anna", MentorID: "u-oleg", Text: text, Status: "open", CreatedAt: e.clock}
	if err := e.st.SaveQuestion(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if code := e.login("oleg").do("POST", "/api/inbox/"+q.ID+"/answer", map[string]any{"answer": answer, "addToFaq": true}, nil); code != 200 {
		t.Fatalf("answer: %d", code)
	}
}

func TestAnswerBecomesFiledDoc(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	answerQuestion(t, e, "Хочу поменять расписание работы, как это сделать?", "напиши руководителю, график меняется с понедельника")
	answerQuestion(t, e, "Где взять ноутбук побольше?", "Оформи заявку в ИТ-поддержку, выдают за 2 дня.")
	answerQuestion(t, e, "Как оформить отпуск летом?", "Заявка за 14 дней, как обычно")
	var list kbList
	e.login("anna").do("GET", "/api/kb", nil, &list)
	if slices.Contains(list.Folders, "Ответы наставников") {
		t.Fatal("legacy folder")
	}
	want := map[string]string{"Хочу поменять расписание работы, как это сделать": "Процессы/График работы",
		"Где взять ноутбук побольше": "Оборудование", "Как оформить отпуск летом": "Процессы/Отсутствия"}
	for title, folder := range want {
		i := slices.IndexFunc(list.Docs, func(d docView) bool { return d.Title == title })
		if i < 0 || list.Docs[i].Folder != folder {
			t.Fatalf("%s → %+v", title, list.Docs)
		}
	}
	var d docView
	e.login("maria").do("GET", "/api/kb/faq-q-Хочуп", nil, &d)
	if d.Source != "answer" || !d.Editable || !strings.Contains(d.Body, "## Коротко\nНапиши руководителю") || !strings.Contains(d.Body, "Олег Кузнецов") {
		t.Fatalf("doc: %+v", d)
	}
	// AI-buddy отвечает по оформленному ответу
	m2, _ := e.login("anna").ask("как поменять расписание работы")
	if !strings.Contains(m2.Text, "понедельника") {
		t.Fatalf("reuse: %s", m2.Text)
	}
}

func TestLegacyFAQMigrated(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	ctx := context.Background()
	_ = e.st.AddFAQ(ctx, domain.FAQEntry{ID: "old1", Team: "search", Question: "Где пропуск на парковку?", Answer: "У охраны на 1 этаже", AuthorID: "u-oleg", CreatedAt: e.clock})
	_ = e.st.AddFAQ(ctx, domain.FAQEntry{ID: "old2", Team: "search", Question: "Где лежит стенд?", Answer: "search-staging", AuthorID: "u-oleg", CreatedAt: e.clock})
	if err := e.app.LoadKB(ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.app.LoadFAQ(ctx); err != nil {
		t.Fatal(err)
	}
	var d docView
	maria := e.login("maria")
	if code := maria.do("GET", "/api/kb/faq-old1", nil, &d); code != 200 || d.Folder != "Офис" || d.Title != "Где пропуск на парковку" {
		t.Fatalf("migrated: %d %+v", code, d)
	}
	if code := maria.do("DELETE", "/api/kb/faq-old2", nil, nil); code != 200 {
		t.Fatal("delete migrated")
	}
	_ = e.app.LoadKB(ctx)
	_ = e.app.LoadFAQ(ctx)
	if code := maria.do("GET", "/api/kb/faq-old2", nil, nil); code != 404 {
		t.Fatal("deleted answer came back")
	}
}

// fakeEditor — модель, которая оформляет инструкцию (и только это).
type fakeEditor struct {
	mu   sync.Mutex
	seen string
	out  string
}

func (f *fakeEditor) Model() string { return "fake" }
func (f *fakeEditor) Chat(_ context.Context, msgs []llm.Message, _ []llm.Tool) (llm.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = msgs[len(msgs)-1].Content
	return llm.Message{Role: "assistant", Content: f.out}, nil
}

func TestAnswerDocPolishedByModel(t *testing.T) {
	model := &fakeEditor{out: "Вот инструкция:\n```json\n" + `{"title":"Как сменить график работы","folder":"Кадры/График","tags":["график","расписание"],"body":"## Как сменить график\n1. Напиши руководителю.\n2. Новый график действует с понедельника.\n\n## Откуда этот ответ\nОлег Кузнецов ответил на вопрос стажёра."}` + "\n```"}
	e := newTestEnvModel(t, allOn(), "", model)
	answerQuestion(t, e, "Хочу поменять расписание работы", "напиши руководителю, график с понедельника")
	e.app.Wait()
	var d docView
	e.login("maria").do("GET", "/api/kb/faq-q-Хочуп", nil, &d)
	if d.Title != "Как сменить график работы" || d.Folder != "Кадры/График" || !strings.Contains(d.Body, "1. Напиши руководителю") {
		t.Fatalf("polished: %+v", d)
	}
	model.mu.Lock()
	if !strings.Contains(model.seen, "Процессы/Отсутствия") || !strings.Contains(model.seen, "Пример инструкции") {
		t.Fatalf("prompt lacks folders/sample: %s", model.seen)
	}
	model.mu.Unlock()
	// ответ модели без JSON — остаётся офлайн-оформление
	model.mu.Lock()
	model.out = "не могу"
	model.mu.Unlock()
	answerQuestion(t, e, "Где взять ноутбук?", "В ИТ-поддержке")
	e.app.Wait()
	e.login("maria").do("GET", "/api/kb/faq-q-Гдевз", nil, &d)
	if d.Folder != "Оборудование" {
		t.Fatalf("fallback: %+v", d)
	}
}
