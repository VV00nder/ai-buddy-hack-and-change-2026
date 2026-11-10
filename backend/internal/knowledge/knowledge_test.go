package knowledge

import (
	"testing"
	"testing/fstest"

	"aibuddy/internal/domain"
)

var (
	anna  = domain.User{ID: "u-anna", Role: domain.RoleIntern, Team: "search"}
	oleg  = domain.User{ID: "u-oleg", Role: domain.RoleMentor, Team: "search"}
	maria = domain.User{ID: "u-maria", Role: domain.RoleLead, Team: "search"}
)

func load(t *testing.T) *Base {
	t.Helper()
	b, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func top(b *Base, q string, u domain.User) string {
	h := b.Search(q, u, 3)
	if len(h) == 0 {
		return ""
	}
	return h[0].DocID
}

func TestSearchRelevance(t *testing.T) {
	b := load(t)
	cases := map[string]string{
		"как оформить отгул?":                       "process-day-off",
		"хочу отпроситься в пятницу":                "process-day-off",
		"стоит ли писать руководителю про выходной": "process-day-off",
		"какие доступы нужны стажёру":               "process-access",
		"как добавить ssh ключ":                     "tool-stash-keys",
		"во сколько дейли":                          "team-search",
		"кто есть кто в команде":                    "team-search",
		"где смотреть ошибки сервиса":               "tool-observability",
		"что такое zero bug policy":                 "process-engineering",
		"как подготовиться к экватору":              "process-equator",
		"не работает avito-cli":                     "tool-avito-cli",
		"можно ли вставлять данные в чат gpt":       "security-data",
	}
	for q, want := range cases {
		if got := top(b, q, anna); got != want {
			t.Errorf("%q: want %s, got %s (%v)", q, want, got, b.Search(q, anna, 3))
		}
	}
	if len(b.Search("", anna, 3)) != 0 || len(b.Search("и в на", anna, 3)) != 0 {
		t.Fatal("empty query")
	}
	if len(b.Search("квантовая хромодинамика", anna, 3)) != 0 {
		t.Fatal("no hits expected")
	}
}

func TestACL(t *testing.T) {
	b := load(t)
	// стажёр не видит методичку наставника и материалы калибровки
	for _, h := range b.Search("методичка наставника калибровка оценка оффер", anna, 10) {
		if h.DocID == "mentor-guide" || h.DocID == "lead-review" {
			t.Fatalf("intern must not see %s", h.DocID)
		}
	}
	if _, ok := b.Get("mentor-guide", anna); ok {
		t.Fatal("get mentor-guide as intern")
	}
	if _, ok := b.Get("mentor-guide", oleg); !ok {
		t.Fatal("mentor must see guide")
	}
	if _, ok := b.Get("lead-review", oleg); ok {
		t.Fatal("mentor must not see lead docs")
	}
	if _, ok := b.Get("lead-review", maria); !ok {
		t.Fatal("lead must see lead docs")
	}
	// чужая команда
	if _, ok := b.Get("team-delivery", anna); ok {
		t.Fatal("foreign team doc")
	}
	if _, ok := b.Get("nope", anna); ok {
		t.Fatal("missing doc")
	}
	for _, d := range b.List(anna) {
		if d.ID == "team-delivery" || d.ID == "mentor-guide" {
			t.Fatal("list leaks")
		}
	}
	if len(b.List(maria)) <= len(b.List(anna)) {
		t.Fatal("lead sees more")
	}
}

func TestFAQ(t *testing.T) {
	b := load(t)
	b.AddFAQ(domain.FAQEntry{ID: "1", Team: "search", Question: "Где лежит стенд для поиска?", Answer: "Стенд search-staging, ссылка в README сервиса"})
	h := b.Search("где стенд поиска", anna, 1)
	if len(h) == 0 || h[0].Kind != "faq" {
		t.Fatalf("faq: %v", h)
	}
	ivan := domain.User{Role: domain.RoleIntern, Team: "delivery"}
	for _, x := range b.Search("где стенд поиска", ivan, 5) {
		if x.Kind == "faq" {
			t.Fatal("faq of another team leaked")
		}
	}
}

func TestParseErrors(t *testing.T) {
	bad := map[string]string{
		"d/a.md": "no front matter",
	}
	for name, body := range bad {
		if _, err := LoadFS(fstest.MapFS{name: {Data: []byte(body)}}, "d"); err == nil {
			t.Fatalf("%s must fail", name)
		}
	}
	if _, err := LoadFS(fstest.MapFS{"d/a.md": {Data: []byte("---\nid: x\n")}}, "d"); err == nil {
		t.Fatal("unterminated")
	}
	if _, err := LoadFS(fstest.MapFS{"d/a.md": {Data: []byte("---\nid: x\n---\ntext")}}, "d"); err == nil {
		t.Fatal("no title")
	}
	if _, err := LoadFS(fstest.MapFS{}, "missing"); err == nil {
		t.Fatal("missing dir")
	}
	b, err := LoadFS(fstest.MapFS{
		"d/a.md":     {Data: []byte("---\r\nid: a\r\ntitle: A\r\n---\r\nintro text\r\n## H\r\nbody\r\n")},
		"d/skip.txt": {Data: []byte("x")},
	}, "d")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := b.Get("a", anna)
	if len(d.Sections) != 2 || d.Team != "all" || d.Audience[0] != "all" {
		t.Fatalf("defaults: %+v", d)
	}
}

func TestStem(t *testing.T) {
	if Stem("доступы") != Stem("доступ") || Stem("отгулы") != Stem("отгул") {
		t.Fatal("stem")
	}
	if Stem("кот") != "кот" || Stem("учиться") == "учиться" {
		t.Fatal("short / reflexive")
	}
	if toks := Tokenize("Day-off, ёлка"); toks[0] != "отгул" {
		t.Fatalf("tokenize: %v", toks)
	}
}
