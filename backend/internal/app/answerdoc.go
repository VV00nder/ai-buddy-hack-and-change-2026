package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"aibuddy/internal/domain"
	"aibuddy/internal/knowledge"
)

// Ответ наставника, который решили сохранить в базу знаний, становится обычной
// инструкцией: AI-buddy приводит его к стилю остальных инструкций и кладёт
// в подходящую папку, а если такой нет — заводит новую. Отдельной папки
// «Ответы наставников» больше нет.

// legacyAnswersFolder — папка, куда раньше складывались ответы наставников.
const legacyAnswersFolder = "Ответы наставников"

// answerCategories — темы, по которым раскладываются ответы, если подходящей папки ещё нет.
var answerCategories = []struct {
	folder string
	words  string
}{
	{"Процессы/График работы", "расписание график рабочий день удалёнка удаленка удалённо офис опоздание начало конец смена гибкий часы переработка"},
	{"Процессы/Отсутствия", "отгул отпуск больничный выходной отсутствие отпроситься"},
	{"Процессы/Доступы", "доступ права роль vpn пароль учётка учетная запись заявка доступ"},
	{"Процессы/Разработка", "ревью релиз деплой тест баг мерж ветка репозиторий код пулреквест"},
	{"Процессы/Стажировка", "стажировка наставник экватор встреча 1:1 план адаптация выпускной"},
	{"Инструменты", "инструмент настройка установить программа cli grafana sentry redash stash ide"},
	{"Оборудование", "ноутбук монитор компьютер мышь клавиатура гарнитура техника принтер"},
	{"Документы и выплаты", "зарплата справка документ договор налог выплата аванс бухгалтерия кадры"},
	{"Коммуникации", "чат почта встреча календарь созвон канал рассылка"},
	{"Офис", "пропуск парковка столовая кухня переговорка этаж охрана"},
	{"Безопасность", "безопасность данные конфиденциальный утечка фишинг персональные"},
}

// answerDraft — инструкция, оформленная из ответа.
type answerDraft struct {
	Title  string   `json:"title"`
	Folder string   `json:"folder"`
	Tags   []string `json:"tags"`
	Body   string   `json:"body"`
}

// teamDocs — инструкции, которые видит отдел (без удалённых и без чужих отделов).
func (a *App) teamDocs(team string) []*knowledge.Doc {
	var out []*knowledge.Doc
	for _, d := range a.kb.List(domain.User{Role: domain.RoleAdmin}) {
		if d.Team == "all" || d.Team == team || team == "all" {
			out = append(out, d)
		}
	}
	return out
}

// pickFolder выбирает папку для ответа: существующую, если она подходит по теме,
// иначе тематическую из answerCategories; по умолчанию — «Общее».
func (a *App) pickFolder(team, question, answer string) string {
	vocab := map[string]map[string]bool{}
	add := func(folder, text string) {
		if folder == "" || folder == legacyAnswersFolder {
			return
		}
		if vocab[folder] == nil {
			vocab[folder] = map[string]bool{}
		}
		for _, t := range knowledge.Tokenize(text) {
			vocab[folder][t] = true
		}
	}
	for _, d := range a.teamDocs(team) {
		if d.Source == "answer" {
			continue // папки ответов уже учтены через темы и названия, их тексты слишком шумные
		}
		add(d.Folder, strings.ReplaceAll(d.Folder, "/", " ")+" "+d.Title+" "+strings.Join(d.Tags, " "))
	}
	for _, c := range answerCategories {
		add(c.folder, strings.ReplaceAll(c.folder, "/", " ")+" "+c.words)
	}
	score := map[string]int{}
	count := func(text string, w int) {
		seen := map[string]bool{}
		for _, t := range knowledge.Tokenize(text) {
			if seen[t] {
				continue
			}
			seen[t] = true
			for f, v := range vocab {
				if v[t] {
					score[f] += w
				}
			}
		}
	}
	count(question, 2) // вопрос точнее описывает тему, чем ответ
	count(answer, 1)
	best, top := "Общее", 0
	folders := make([]string, 0, len(score))
	for f := range score {
		folders = append(folders, f)
	}
	sort.Strings(folders)
	for _, f := range folders {
		if score[f] > top {
			best, top = f, score[f]
		}
	}
	return best
}

var tagStop = map[string]bool{"хочу": true, "можно": true, "нужно": true, "надо": true, "какой": true, "какая": true, "какие": true,
	"когда": true, "where": true, "почему": true, "сколько": true, "чтобы": true, "если": true, "меня": true, "мне": true, "есть": true,
	"было": true, "будет": true, "через": true, "после": true, "перед": true, "этот": true, "это": true, "тоже": true, "ещё": true, "еще": true}

// questionTitle превращает вопрос в название инструкции: без «?», с заглавной буквы.
func questionTitle(q string) string {
	q = strings.Join(strings.Fields(q), " ")
	q = strings.TrimRight(q, "?!. ")
	if utf8.RuneCountInString(q) > 140 {
		q = string([]rune(q)[:140]) + "…"
	}
	if utf8.RuneCountInString(q) < 3 {
		q = "Ответ на вопрос стажёра"
	}
	return upperFirst(q)
}

func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// offlineAnswerDoc — оформление без языковой модели: название из вопроса,
// разделы как в остальных инструкциях, папка — по теме.
func (a *App) offlineAnswerDoc(team, question, answer, who string, at time.Time) answerDraft {
	title := questionTitle(question)
	body := strings.TrimSpace(strings.ReplaceAll(answer, "\r\n", "\n"))
	body = upperFirst(body)
	if r, _ := utf8.DecodeLastRuneInString(body); r != utf8.RuneError && !strings.ContainsRune(".!?…:)»`*", r) && !strings.Contains(body, "\n") {
		body += "."
	}
	text := "## Коротко\n" + body + "\n\n## Откуда этот ответ\n" +
		fmt.Sprintf("%s ответил(а) на вопрос стажёра %s: «%s». Если что-то изменилось — поправьте инструкцию.", who, at.Format("02.01.2006"), strings.TrimSpace(question))
	var tags []string
	for _, w := range strings.FieldsFunc(strings.ToLower(question), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if utf8.RuneCountInString(w) >= 4 && !tagStop[w] && !contains(tags, w) && len(tags) < 6 {
			tags = append(tags, w)
		}
	}
	return answerDraft{Title: title, Folder: a.pickFolder(team, question, answer), Tags: tags, Body: text}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

const answerDocPrompt = `Ты — редактор базы знаний отдела. Наставник ответил стажёру на вопрос, ответ решили сохранить как инструкцию.
Оформи его в том же стиле, что и остальные инструкции базы:
- название — утвердительное, по сути, без «Ответ наставника» и без вопросительного знака (пример: «Как оформить отгул или отпуск»);
- текст в markdown, разделы начинаются строкой «## Заголовок», шаги — нумерованным списком, обращение на «ты», коротко и по делу;
- не добавляй фактов, которых нет в ответе, не выдумывай сроки, ссылки и имена; если ответ короткий — инструкция тоже короткая;
- в конце раздел «## Откуда этот ответ» одной строкой: кто ответил и на какой вопрос;
- папка: выбери самую подходящую из списка существующих; если ни одна не подходит — придумай новую короткую категорию (1–2 уровня через «/», например «Процессы/График работы»); папку «Ответы наставников» не используй;
- 3–6 тегов в нижнем регистре.
Верни только JSON без пояснений: {"title":"…","folder":"…","tags":["…"],"body":"…"}`

// llmAnswerDoc просит модель оформить инструкцию; ошибка — оставляем офлайн-вариант.
func (a *App) llmAnswerDoc(ctx context.Context, team, question, answer, who string, at time.Time) (answerDraft, error) {
	var folders []string
	seen := map[string]bool{}
	var sample string
	for _, d := range a.teamDocs(team) {
		if d.Folder != "" && d.Folder != legacyAnswersFolder && !seen[d.Folder] {
			seen[d.Folder] = true
			folders = append(folders, d.Folder)
		}
		if sample == "" && d.Source != "answer" && len(d.Sections) > 1 {
			var b strings.Builder
			for _, s := range d.Sections[:min(2, len(d.Sections))] {
				fmt.Fprintf(&b, "## %s\n%s\n\n", s.Heading, s.Text)
			}
			sample = "# " + d.Title + "\n" + b.String()
		}
	}
	sort.Strings(folders)
	user := fmt.Sprintf("Существующие папки:\n- %s\n\nПример инструкции из базы:\n%s\nВопрос стажёра: %s\nОтвет (%s, %s):\n%s",
		strings.Join(folders, "\n- "), sample, question, who, at.Format("02.01.2006"), answer)
	out, err := a.agent.Complete(ctx, answerDocPrompt, user)
	if err != nil {
		return answerDraft{}, err
	}
	i, j := strings.Index(out, "{"), strings.LastIndex(out, "}")
	if i < 0 || j <= i {
		return answerDraft{}, fmt.Errorf("answer doc: no json")
	}
	var d answerDraft
	if err := json.Unmarshal([]byte(out[i:j+1]), &d); err != nil {
		return answerDraft{}, err
	}
	d.Folder = knowledge.CleanFolder(d.Folder)
	if d.Folder == "" || d.Folder == legacyAnswersFolder || strings.Count(d.Folder, "/") > 2 {
		d.Folder = a.pickFolder(team, question, answer)
	}
	return d, nil
}

// saveAnswerDoc сохраняет ответ как инструкцию отдела. Сразу — офлайн-оформление,
// чтобы AI-buddy начал отвечать по ней без задержки; если подключена модель,
// она в фоне переписывает инструкцию в общем стиле и уточняет папку.
func (a *App) saveAnswerDoc(ctx context.Context, f domain.FAQEntry, who string, polish bool) error {
	id := "faq-" + f.ID
	d := a.offlineAnswerDoc(f.Team, f.Question, f.Answer, who, f.CreatedAt)
	rec, err := a.putAnswerDoc(ctx, id, f.Team, who, d)
	if err != nil || !polish || a.agent == nil || a.agent.Mode() != "llm" {
		return err
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
		defer cancel()
		better, err := a.llmAnswerDoc(bg, f.Team, f.Question, f.Answer, who, f.CreatedAt)
		if err != nil {
			a.log.Warn("answer doc: llm", "err", err)
			return
		}
		// если инструкцию уже поправили руками или удалили — не трогаем
		if cur, err := getRec[KBDoc](bg, a.store, recKBDoc, id); err != nil || cur.UpdatedAt != rec.UpdatedAt {
			return
		}
		if _, err := a.putAnswerDoc(bg, id, f.Team, who, better); err != nil {
			a.log.Warn("answer doc: save", "err", err)
		}
	}()
	return nil
}

func (a *App) putAnswerDoc(ctx context.Context, id, team, who string, d answerDraft) (KBDoc, error) {
	in, err := cleanDocInput(DocInput{Title: d.Title, Folder: d.Folder, Audience: []string{"all"}, Tags: d.Tags, Body: d.Body})
	if err != nil {
		return KBDoc{}, err
	}
	rec := KBDoc{ID: id, Title: in.Title, Folder: in.Folder, Audience: in.Audience, Team: team, Tags: in.Tags, Body: in.Body,
		UpdatedBy: who, UpdatedAt: a.now().Format(time.RFC3339Nano), Source: "answer"}
	if err := putRec(ctx, a.store, recKBDoc, id, rec); err != nil {
		return rec, err
	}
	a.kb.Upsert(rec.toDoc())
	return rec, nil
}
