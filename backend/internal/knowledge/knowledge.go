// Package knowledge — база знаний команды с учётом прав доступа и
// поиском BM25 по разделам документов.
//
// Права проверяются до того, как текст попадает в контекст LLM: документ,
// недоступный роли или команде пользователя, не может «утечь» в ответ даже
// при попытке prompt injection.
package knowledge

import (
	"embed"
	"fmt"
	"io/fs"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode"

	"aibuddy/internal/domain"
)

//go:embed docs/*.md
var docsFS embed.FS

// Doc — документ базы знаний.
type Doc struct {
	ID        string
	Title     string
	Audience  []string // роли; "all" — все
	Team      string   // id команды или "all"
	Tags      []string
	Sections  []Section
	Synthetic bool
}

// Section — раздел документа (единица поиска).
type Section struct {
	Heading string `json:"heading"`
	Text    string `json:"text"`
}

// Hit — результат поиска.
type Hit struct {
	DocID   string  `json:"docId"`
	Title   string  `json:"title"`
	Section string  `json:"section"`
	Text    string  `json:"text"`
	Score   float64 `json:"score"`
	Kind    string  `json:"kind"` // doc | faq
	// Coverage — доля слов запроса, найденных в разделе (0..1).
	Coverage float64 `json:"coverage"`
}

type chunk struct {
	doc     *Doc
	section Section
	terms   map[string]int
	length  int
	kind    string
	faqTeam string
}

// Base — индекс документов.
type Base struct {
	mu     sync.RWMutex
	docs   map[string]*Doc
	chunks []*chunk
	df     map[string]int
	avgLen float64
}

// Load загружает встроенные документы.
func Load() (*Base, error) { return LoadFS(docsFS, "docs") }

// LoadFS загружает документы из файловой системы.
func LoadFS(fsys fs.FS, dir string) (*Base, error) {
	b := &Base{docs: map[string]*Doc{}}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := fs.ReadFile(fsys, dir+"/"+e.Name())
		if err != nil {
			return nil, err
		}
		d, err := parse(string(raw))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		b.docs[d.ID] = d
		for _, s := range d.Sections {
			b.addChunk(&chunk{doc: d, section: s, kind: "doc"})
		}
	}
	b.reindex()
	return b, nil
}

func parse(raw string) (*Doc, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(raw, "---\n") {
		return nil, fmt.Errorf("no front matter")
	}
	head, body, ok := strings.Cut(raw[4:], "\n---\n")
	if !ok {
		return nil, fmt.Errorf("unterminated front matter")
	}
	d := &Doc{Synthetic: true}
	for _, line := range strings.Split(head, "\n") {
		k, v, _ := strings.Cut(line, ":")
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "id":
			d.ID = v
		case "title":
			d.Title = v
		case "audience":
			d.Audience = splitList(v)
		case "team":
			d.Team = v
		case "tags":
			d.Tags = splitList(v)
		}
	}
	if d.ID == "" || d.Title == "" {
		return nil, fmt.Errorf("id and title are required")
	}
	if d.Team == "" {
		d.Team = "all"
	}
	if len(d.Audience) == 0 {
		d.Audience = []string{"all"}
	}
	var cur *Section
	intro := Section{Heading: d.Title}
	for _, line := range strings.Split(body, "\n") {
		if h, ok := strings.CutPrefix(line, "## "); ok {
			if cur != nil {
				d.Sections = append(d.Sections, *cur)
			}
			cur = &Section{Heading: strings.TrimSpace(h)}
			continue
		}
		if cur == nil {
			intro.Text += line + "\n"
		} else {
			cur.Text += line + "\n"
		}
	}
	if cur != nil {
		d.Sections = append(d.Sections, *cur)
	}
	if strings.TrimSpace(intro.Text) != "" {
		d.Sections = append([]Section{intro}, d.Sections...)
	}
	for i := range d.Sections {
		d.Sections[i].Text = strings.TrimSpace(d.Sections[i].Text)
	}
	return d, nil
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.ToLower(p))
		}
	}
	return out
}

func (b *Base) addChunk(c *chunk) {
	text := c.section.Heading + " " + c.section.Text
	if c.doc != nil {
		text += " " + c.doc.Title + " " + strings.Join(c.doc.Tags, " ")
	}
	c.terms = map[string]int{}
	for _, t := range Tokenize(text) {
		c.terms[t]++
		c.length++
	}
	b.chunks = append(b.chunks, c)
}

func (b *Base) reindex() {
	b.df = map[string]int{}
	total := 0
	for _, c := range b.chunks {
		total += c.length
		for t := range c.terms {
			b.df[t]++
		}
	}
	if len(b.chunks) > 0 {
		b.avgLen = float64(total) / float64(len(b.chunks))
	}
}

// AddFAQ добавляет ответ наставника в базу знаний команды.
func (b *Base) AddFAQ(e domain.FAQEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d := &Doc{ID: "faq-" + e.ID, Title: "Ответ наставника: " + e.Question, Audience: []string{"all"}, Team: e.Team}
	d.Sections = []Section{{Heading: e.Question, Text: e.Answer}}
	b.docs[d.ID] = d
	b.addChunk(&chunk{doc: d, section: d.Sections[0], kind: "faq"})
	b.reindex()
}

// CanRead проверяет право пользователя на документ.
func CanRead(d *Doc, u domain.User) bool {
	if d.Team != "all" && d.Team != u.Team {
		return false
	}
	return slices.Contains(d.Audience, "all") || slices.Contains(d.Audience, string(u.Role))
}

// Get возвращает документ, если он доступен пользователю.
func (b *Base) Get(id string, u domain.User) (*Doc, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	d, ok := b.docs[id]
	if !ok || !CanRead(d, u) {
		return nil, false
	}
	return d, true
}

// List возвращает доступные пользователю документы.
func (b *Base) List(u domain.User) []*Doc {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var out []*Doc
	for _, d := range b.docs {
		if CanRead(d, u) {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

// Search ищет по доступным пользователю разделам (BM25).
func (b *Base) Search(query string, u domain.User, limit int) []Hit {
	b.mu.RLock()
	defer b.mu.RUnlock()
	q := Tokenize(query)
	if len(q) == 0 {
		return nil
	}
	uniq := map[string]bool{}
	for _, t := range q {
		uniq[t] = true
	}
	const k1, bb = 1.4, 0.75
	n := float64(len(b.chunks))
	var hits []Hit
	for _, c := range b.chunks {
		if !CanRead(c.doc, u) {
			continue
		}
		score := 0.0
		matched := map[string]bool{}
		for _, t := range q {
			tf := float64(c.terms[t])
			if tf == 0 {
				continue
			}
			matched[t] = true
			idf := math.Log(1 + (n-float64(b.df[t])+0.5)/(float64(b.df[t])+0.5))
			score += idf * tf * (k1 + 1) / (tf + k1*(1-bb+bb*float64(c.length)/b.avgLen))
		}
		if c.kind == "faq" {
			score *= 1.2 // ответы наставников команды — самые точные
		}
		if score > 0 {
			hits = append(hits, Hit{DocID: c.doc.ID, Title: c.doc.Title, Section: c.section.Heading, Text: c.section.Text,
				Score: score, Kind: c.kind, Coverage: float64(len(matched)) / float64(len(uniq))})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

var stop = map[string]bool{}

func init() {
	for _, w := range strings.Fields("и в во не что он на я с со как а то все она так его но да ты к у же вы за бы по только ее мне было вот от меня еще нет о из ему теперь когда даже ну вдруг ли если уже или ни быть был него до вас нибудь опять уж вам ведь там потом себя ничего ей может они тут где есть надо ней для мы тебя их чем была сам чтоб без будто чего раз тоже себе под будет ж тогда кто этот того потому этого какой совсем ним здесь этом один почти мой тем чтобы нее сейчас были куда зачем всех никогда можно при наконец два об другой хоть после над больше тот через эти нас про всего них какая много разве три эту моя впрочем хорошо свою этой перед иногда лучше чуть том нельзя такой им более всегда конечно всю между это мне мной мою моё мое мои как-то какие каком подскажи подскажите скажи расскажи пожалуйста привет нужно нужен нужна хочу мне можно ли the a to is") {
		stop[w] = true
	}
}

// synonyms приводит синонимы к общей основе (после стемминга).
var synonyms = map[string]string{
	"отпрос": "отгул", "выходн": "отгул", "отсутств": "отгул", "dayoff": "отгул",
	"прав": "доступ", "access": "доступ", "бадд": "наставник", "buddy": "наставник", "ментор": "наставник",
	"руководител": "руковод", "лид": "руковод", "тимлид": "руковод",
	"bitbucket": "stash", "гит": "stash", "репозитор": "stash", "ssh": "ключ",
	"метрик": "grafana", "дашборд": "grafana", "ошибк": "sentry", "баг": "zbp",
}

var suffixes = []string{
	"иями", "ями", "ами", "ией", "иях", "ого", "его", "ому", "ему", "ыми", "ими", "ость", "ости",
	"ешь", "ете", "ишь", "ите", "ала", "ала", "ила", "ыла", "ать", "ять", "ить", "еть", "уть",
	"ый", "ий", "ой", "ая", "яя", "ое", "ее", "ые", "ие", "ов", "ев", "ей", "ам", "ям", "ах", "ях",
	"ом", "ем", "ую", "юю", "ию", "ия", "ье", "ья", "ть", "ся", "сь", "ет", "ит", "ут", "ют", "ат", "ят",
	"а", "я", "о", "е", "ы", "и", "у", "ю", "ь", "й",
}

// Stem — облегчённый стеммер для русского языка.
func Stem(w string) string {
	w = strings.ReplaceAll(strings.ToLower(w), "ё", "е")
	r := []rune(w)
	if len(r) <= 3 {
		return w
	}
	for _, s := range suffixes {
		if strings.HasSuffix(w, s) && len([]rune(w))-len([]rune(s)) >= 3 {
			w = strings.TrimSuffix(w, s)
			break
		}
	}
	if strings.HasSuffix(w, "ся") || strings.HasSuffix(w, "сь") {
		w = w[:len(w)-len("ся")]
	}
	return w
}

// Tokenize разбивает текст на нормализованные термы.
func Tokenize(text string) []string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
	})
	var out []string
	for _, w := range words {
		w = strings.Trim(w, "-")
		if w == "" || stop[w] {
			continue
		}
		if w == "day-off" {
			w = "dayoff"
		}
		st := Stem(w)
		for prefix, canon := range synonyms {
			if strings.HasPrefix(st, prefix) {
				st = canon
				break
			}
		}
		out = append(out, st)
	}
	return out
}
