package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"aibuddy/internal/dateparse"
	"aibuddy/internal/domain"
	"aibuddy/internal/knowledge"
)

// minScore — порог уверенности поиска: ниже него ответ не считается найденным.
const minScore = 2.2

// minCoverage — минимальная доля слов вопроса, найденных в разделе.
const minCoverage = 0.5

func has(t string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(t, w) {
			return true
		}
	}
	return false
}

var systemAliases = []struct {
	id    string
	words []string
}{
	{"k8s-staging", []string{"staging", "стейдж", "стэйдж", "тестовый кластер", "стенд"}},
	{"k8s-prod", []string{"kubernetes", "k8s", "кубер", "продов"}},
	{"confluence-cluster", []string{"кластер"}},
	{"channel-tech-dep", []string{"tech-dep", "techdep", "техдеп", "tech dep", "тех деп"}},
	{"confluence-team", []string{"confluence", "конфлюенс", "конфлю", "вики"}},
	{"stash", []string{"stash", "стэш", "стеш", "репозитор", "bitbucket", "гит"}},
	{"vpn", []string{"vpn", "впн"}},
	{"jira", []string{"jira", "джир"}},
	{"grafana", []string{"grafana", "графан"}},
	{"sentry", []string{"sentry", "сентри"}},
	{"redash", []string{"redash", "редаш"}},
}

var reProject = regexp.MustCompile(`\b(search-api|search-indexer|search-web|delivery-tracking)\b`)

// detectLevel — уровень прав из текста: чтение, редактирование, администрирование.
func detectLevel(t string) string {
	switch {
	case has(t, "админ", "admin"):
		return "admin"
	case has(t, "запис", "write", "редакт", "edit", "пуш", "push", "изменен"):
		return "edit"
	case has(t, "чтени", "read", "просмотр", "только смотреть"):
		return "read"
	}
	return ""
}

func detectSystem(t string) string {
	for _, s := range systemAliases {
		if has(t, s.words...) {
			return s.id
		}
	}
	return ""
}

var reJustification = regexp.MustCompile(`(?:для|чтобы|потому что|так как|нужно для)\s+(.{10,})`)

func (a *Agent) handleOffline(ctx context.Context, req Request) Reply {
	t := strings.ToLower(strings.ReplaceAll(req.Text, "ё", "е"))
	awaiting := map[string]string{}
	if n := len(req.History); n > 0 {
		for i := n - 1; i >= 0; i-- {
			if req.History[i].Author == "assistant" {
				awaiting = req.History[i].Context
				break
			}
		}
	}
	a.step(req, "thinking", "Разбираю вопрос")
	switch {
	case awaiting["awaiting"] == "absence_date" && !has(t, "доступ", "наставник", "план"):
		if r, ok := dateparse.Parse(t, a.d.Now()); ok {
			return a.absenceWithDates(ctx, req, r, awaiting["type"])
		}
	case awaiting["awaiting"] == "access_justification":
		if utf8.RuneCountInString(strings.TrimSpace(req.Text)) >= 15 {
			return a.accessFlow(ctx, req, accessReq{awaiting["system"], awaiting["project"], awaiting["level"], strings.TrimSpace(req.Text)})
		}
	case awaiting["awaiting"] == "access_project":
		if p := reProject.FindString(t); p != "" {
			return a.accessFlow(ctx, req, accessReq{awaiting["system"], p, awaiting["level"], awaiting["justification"]})
		}
	}
	switch {
	case has(t, "спроси наставник", "передай наставник", "задай вопрос наставник", "спросить наставник", "вопрос наставнику", "спроси у наставник", "спроси бадди"):
		q := req.Text
		if i := strings.Index(t, ":"); i >= 0 && i+1 < len(req.Text) {
			q = req.Text[i+1:]
		}
		return Reply{Text: "Подготовил вопрос наставнику. Проверь формулировку и отправь — ответ придёт в уведомления.",
			Proposal: prepareMentorQuestion(q), Answered: true}
	case has(t, "что ты умеешь", "что умеешь", "помощь", "help", "/start") || regexp.MustCompile(`^(привет|здравств|добрый|хай)`).MatchString(t):
		return a.greet(ctx, req)
	case isTeamIntent(req.User, t):
		return a.teamReply(ctx, req)
	case isQuizIntent(t):
		return a.quizReply(ctx, req)
	case isStepDoneIntent(t):
		return a.stepDone(ctx, req)
	case has(t, "статус") || has(t, "что с моей", "что с заявк", "одобрил", "согласовал", "мои заявки"):
		return a.statuses(ctx, req)
	case has(t, "отгул", "отпуск", "выходн", "отпрос", "day off", "dayoff", "не выйду", "не смогу выйти", "отсутств"):
		return a.absenceFlow(ctx, req, t)
	case has(t, "доступ", "права ", "прав к", "access") || (detectSystem(t) != "" && !has(t, "как ", "что такое", "зачем") && has(t, "дай", "выдай", "запроси", "нужен", "нужна", "добавь", "хочу")):
		return a.accessIntent(ctx, req, t)
	case a.d.Goals != nil && has(t, "мои цели", "у меня цели", "какие цели", "цели на стажир", "цели стажир", "цель стажир", "критери", "ожидания от меня", "что от меня ждут", "мои задачи на стажир"):
		return a.goalsReply(ctx, req)
	case has(t, "что дальше", "что делать", "что мне делать", "следующий шаг", "следующ", "мой план", "план адаптации", "с чего начать", "что осталось", "прогресс"):
		return a.planReply(ctx, req)
	case has(t, "мой наставник", "мой бадди", "мой руководитель", "мой лид", "к кому обратиться", "кто в команде", "кто мой", "кто моя", "кто есть кто", "контакты"):
		return a.whoReply(ctx, req)
	}
	return a.kbAnswer(ctx, req)
}

func (a *Agent) greet(ctx context.Context, req Request) Reply {
	text := fmt.Sprintf("Привет, %s! Я AI-buddy — помогаю освоиться в команде. Что я умею:\n"+
		"- отвечать на вопросы о команде, процессах и инструментах со ссылками на инструкции;\n"+
		"- показывать план адаптации и подсказывать следующий шаг;\n"+
		"- готовить заявки на отгул и доступы — ты только подтверждаешь;\n"+
		"- передавать вопросы наставнику, если в базе знаний нет ответа;\n"+
		"- отмечать выполненные шаги плана и предлагать тесты, чтобы проверить себя.", firstName(req.User.Name))
	r := Reply{Text: text, Answered: true, Suggestions: []string{"Что мне делать дальше?", "Какие доступы мне нужны?", "Как оформить отгул?"}}
	if a.d.Plan != nil {
		if p, err := a.d.Plan(ctx, req.User); err == nil && p.Next != nil {
			r.Text += fmt.Sprintf("\n\nСейчас у тебя %d-й день стажировки. Следующий шаг: **%s**.", p.Day, p.Next.Title)
		}
	}
	return r
}

func firstName(full string) string {
	if f := strings.Fields(full); len(f) > 0 {
		return f[0]
	}
	return "коллега"
}

// earliestDayOff — ближайшая дата, на которую можно оформить отгул (за 2 рабочих дня).
func earliestDayOff(now time.Time) time.Time {
	d := now
	for n := 2; n > 0; {
		d = d.AddDate(0, 0, 1)
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			n--
		}
	}
	return d
}

func (a *Agent) absenceFlow(ctx context.Context, req Request, t string) Reply {
	kind := "day_off"
	if has(t, "отпуск") && !has(t, "отпуск за свой") {
		kind = "vacation"
	}
	r, hasDate := dateparse.Parse(t, a.d.Now())
	if hasDate {
		return a.absenceWithDates(ctx, req, r, kind)
	}
	if has(t, "сколько", "баланс", "осталось") {
		return a.balance(ctx, req)
	}
	earliest := earliestDayOff(a.d.Now()).Format("2006-01-02")
	asksHow := has(t, "как ", "стоит ли", "нужно ли", "надо ли", "правил", "можно ли", "сколько", "что нужно", "порядок", "отвлека")
	if asksHow {
		r := a.absenceHowTo(req, t)
		r.Text += fmt.Sprintf("\n\nМогу сразу подготовить заявку. Ближайшая дата по правилам — %s.", ruDate(earliest))
		r.Suggestions = []string{"Оформи отгул на " + ruDate(earliest), "Сколько у меня отгулов?"}
		r.Context = map[string]string{"awaiting": "absence_date", "type": kind}
		return r
	}
	return Reply{Text: fmt.Sprintf("Помогу оформить %s. На какую дату? Можно написать «в пятницу», «13.11» или «с 16 по 17 ноября». Ближайшая дата по правилам — %s.",
		map[string]string{"day_off": "отгул", "vacation": "отпуск"}[kind], ruDate(earliest)),
		Suggestions: []string{ruDate(earliest), "Как оформить отгул?"}, Answered: true,
		Context: map[string]string{"awaiting": "absence_date", "type": kind}}
}

// absenceHowTo — справка по порядку оформления из инструкции: сама процедура
// и разделы, о которых спросили (руководитель, отпуск, больничный).
func (a *Agent) absenceHowTo(req Request, t string) Reply {
	a.step(req, "tool", toolLabel("kb_search"))
	doc, ok := a.d.KB.Get("process-day-off", req.User)
	if !ok {
		return a.kbAnswer(context.Background(), req)
	}
	want := map[string]bool{"Отгул": true}
	if has(t, "отвлека", "руковод", "писать", "лид") {
		want["Нужно ли отвлекать руководителя?"] = true
	}
	if has(t, "отпуск") {
		want["Отпуск"] = true
	}
	if has(t, "болез", "больнич", "заболел") {
		want["Больничный"] = true
	}
	var b strings.Builder
	var src []domain.Source
	for _, sec := range doc.Sections {
		if want[sec.Heading] {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			fmt.Fprintf(&b, "**%s**\n%s", sec.Heading, sec.Text)
			src = append(src, domain.Source{DocID: doc.ID, Title: doc.Title, Section: sec.Heading})
		}
	}
	return Reply{Text: b.String(), Sources: src, Answered: true}
}

func (a *Agent) balance(ctx context.Context, req Request) Reply {
	a.step(req, "tool", toolLabel("absence_balance"))
	b, err := a.d.Corp.Call(ctx, req.Token, "absence_balance", nil)
	if err != nil {
		return Reply{Text: unavailableText("отсутствий")}
	}
	return Reply{Text: fmt.Sprintf("У тебя осталось дней отгула: **%v**.", b["day_off_left"]), Answered: true}
}

func (a *Agent) absenceWithDates(ctx context.Context, req Request, r dateparse.Range, kind string) Reply {
	if kind == "" {
		kind = "day_off"
	}
	args := map[string]any{"type": kind, "date_from": r.From.Format("2006-01-02"), "date_to": r.To.Format("2006-01-02")}
	p, ce, err := a.prepareAbsence(ctx, req, args)
	if err != nil {
		return Reply{Text: unavailableText("отсутствий"), Suggestions: []string{req.Text}}
	}
	if ce != nil {
		reply := Reply{Text: "Не получится: " + explainCallError(ce), Answered: true, Context: map[string]string{"awaiting": "absence_date", "type": kind}}
		if ce.Code == "too_late" || ce.Code == "past_date" || ce.Code == "weekend" {
			reply.Suggestions = []string{"Оформи отгул на " + ruDate(earliestDayOff(a.d.Now()).Format("2006-01-02"))}
		}
		return reply
	}
	text := fmt.Sprintf("Проверил по правилам — всё в порядке. Подготовил заявку: **%s**, рабочих дней: %v. Согласует %v. После отправки статус появится в уведомлениях.",
		p.Title, p.Preview["days"], p.Preview["approver"])
	return Reply{Text: text, Proposal: p, Answered: true}
}

func (a *Agent) accessIntent(ctx context.Context, req Request, t string) Reply {
	sys := detectSystem(t)
	if sys == "" {
		return a.accessOverview(ctx, req)
	}
	ar := accessReq{system: sys, project: reProject.FindString(t), level: detectLevel(t)}
	if m := reJustification.FindStringSubmatch(req.Text); m != nil {
		ar.justification = strings.TrimSpace(m[1])
	}
	return a.accessFlow(ctx, req, ar)
}

type accessReq struct{ system, project, level, justification string }

func (ar accessReq) context(waitFor string) map[string]string {
	return map[string]string{"awaiting": waitFor, "system": ar.system, "project": ar.project, "level": ar.level, "justification": ar.justification}
}

func (a *Agent) accessFlow(ctx context.Context, req Request, ar accessReq) Reply {
	args := map[string]any{"system": ar.system}
	for k, v := range map[string]string{"project": ar.project, "level": ar.level, "justification": ar.justification} {
		if v != "" {
			args[k] = v
		}
	}
	p, ce, err := a.prepareAccess(ctx, req, args)
	if err != nil {
		return Reply{Text: unavailableText("доступов"), Suggestions: []string{req.Text}}
	}
	if ce != nil {
		r := Reply{Text: explainCallError(ce), Answered: true}
		switch ce.Code {
		case "justification_required":
			r.Context = ar.context("access_justification")
			r.Text += "\n\nНапиши одним сообщением, для какой задачи нужен доступ."
		case "project_required", "unknown_project":
			r.Context = ar.context("access_project")
			_, list, _ := strings.Cut(ce.Message, ": ")
			for _, p := range strings.Split(list, ", ") {
				if p != "" {
					r.Suggestions = append(r.Suggestions, p)
				}
			}
			r.Text = "К какому проекту нужен доступ? " + ce.Message
		case "level_not_allowed":
			r.Suggestions = []string{"Запроси доступ к " + ar.system + " " + ar.project + " на редактирование"}
		case "duplicate":
			r.Suggestions = []string{"Статус моих заявок"}
		}
		return r
	}
	text := fmt.Sprintf("Подготовил заявку на доступ: **%s**, уровень — %v. %v.", p.Title, p.Preview["level_name"], p.Preview["expected"])
	return Reply{Text: text, Proposal: p, Answered: true}
}

func (a *Agent) accessOverview(ctx context.Context, req Request) Reply {
	a.step(req, "tool", toolLabel("access_catalog"))
	cat, err := a.d.Corp.Call(ctx, req.Token, "access_catalog", nil)
	if err != nil {
		return Reply{Text: unavailableText("доступов")}
	}
	hits := a.d.KB.Search("какие доступы нужны стажеру первая неделя", req.User, 1)
	var granted, pending, missing []string
	var sugg []string
	items, _ := cat["items"].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m["available"] == false {
			continue
		}
		name, _ := m["name"].(string)
		switch m["myStatus"] {
		case "granted":
			granted = append(granted, name)
		case "pending":
			pending = append(pending, name)
		default:
			label := name
			if role, _ := m["approverRole"].(string); role != "" {
				label += " (согласует " + role + ")"
			}
			missing = append(missing, label)
			if len(sugg) < 3 {
				sugg = append(sugg, "Запроси доступ к "+name)
			}
		}
	}
	var b strings.Builder
	b.WriteString("Базовый набор первой недели: VPN, Jira, Confluence команды и канал Tech-dep — выдаются сразу. Репозитории Stash согласует владелец проекта, пространство кластера — руководитель, тестовый Kubernetes — дежурный DevOps.\n")
	if len(granted) > 0 {
		b.WriteString("\n**Уже есть:** " + strings.Join(granted, ", "))
	}
	if len(pending) > 0 {
		b.WriteString("\n**Ждут согласования:** " + strings.Join(pending, ", "))
	}
	if len(missing) > 0 {
		b.WriteString("\n**Можно запросить:** " + strings.Join(missing, "; "))
	}
	b.WriteString("\n\nНапиши, какой доступ запросить, — я проверю заявку и покажу, кто согласует.")
	return Reply{Text: b.String(), Sources: sourcesOf(hits), Suggestions: sugg, Answered: true}
}

func (a *Agent) statuses(ctx context.Context, req Request) Reply {
	a.step(req, "tool", toolLabel("absence_list"))
	abs, err1 := a.d.Corp.Call(ctx, req.Token, "absence_list", map[string]any{"scope": "mine"})
	a.step(req, "tool", toolLabel("access_list"))
	acc, err2 := a.d.Corp.Call(ctx, req.Token, "access_list", map[string]any{"scope": "mine"})
	if err1 != nil && err2 != nil {
		return Reply{Text: unavailableText("заявок")}
	}
	statusRu := map[string]string{"pending": "ждёт согласования", "approved": "согласована", "rejected": "отклонена", "granted": "выдан"}
	var lines []string
	if items, ok := abs["items"].([]any); ok {
		for _, it := range items {
			m := it.(map[string]any)
			lines = append(lines, fmt.Sprintf("- %s (%v): **%s**%s", absenceTitle(m2args(m)), m["id"], statusRu[fmt.Sprint(m["status"])], comment(m)))
		}
	}
	if items, ok := acc["items"].([]any); ok {
		for _, it := range items {
			m := it.(map[string]any)
			lines = append(lines, fmt.Sprintf("- Доступ: %v (%v): **%s**%s", m["systemName"], m["id"], statusRu[fmt.Sprint(m["status"])], comment(m)))
		}
	}
	if len(lines) == 0 {
		return Reply{Text: "У тебя пока нет заявок. Могу подготовить заявку на отгул или доступ.", Answered: true,
			Suggestions: []string{"Какие доступы мне нужны?", "Как оформить отгул?"}}
	}
	return Reply{Text: "Твои заявки:\n" + strings.Join(lines, "\n"), Answered: true}
}

func m2args(m map[string]any) map[string]any {
	return map[string]any{"type": m["type"], "date_from": m["dateFrom"], "date_to": m["dateTo"]}
}

func comment(m map[string]any) string {
	if c, _ := m["comment"].(string); c != "" {
		return " — «" + c + "»"
	}
	return ""
}

func (a *Agent) planReply(ctx context.Context, req Request) Reply {
	a.step(req, "tool", toolLabel("plan_get"))
	p, err := a.d.Plan(ctx, req.User)
	if err != nil {
		return Reply{Text: "Не получилось загрузить план адаптации. Попробуй обновить страницу."}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Сегодня %d-й день стажировки. Выполнено %d из %d шагов (%d%%).", p.Day, p.Done, p.Total, p.Percent)
	if p.Overdue > 0 {
		fmt.Fprintf(&b, " Просрочено шагов: %d — давай начнём с них.", p.Overdue)
	}
	r := Reply{Answered: true}
	if p.Next != nil {
		fmt.Fprintf(&b, "\n\n**Следующий шаг:** %s\n%s", p.Next.Title, p.Next.Description)
		if p.Next.Prompt != "" {
			r.Suggestions = append(r.Suggestions, p.Next.Prompt)
		}
	}
	var rest []string
	for _, per := range p.Periods {
		if per.ID != p.CurrentPeriod {
			continue
		}
		for _, s := range per.Steps {
			if s.Owner == "intern" && s.Status != domain.StepDone && (p.Next == nil || s.ID != p.Next.ID) {
				rest = append(rest, "- "+s.Title)
			}
		}
	}
	if len(rest) > 0 {
		if len(rest) > 5 {
			rest = append(rest[:5], fmt.Sprintf("- и ещё %d", len(rest)-5))
		}
		b.WriteString("\n\nЕщё на этом этапе:\n" + strings.Join(rest, "\n"))
	}
	r.Text = b.String()
	r.Suggestions = append(r.Suggestions, "Открыть мой план")
	return r
}

var goalStatusRu = map[domain.GoalStatus]string{domain.GoalAgreed: "согласована", domain.GoalInProgress: "в работе", domain.GoalDone: "выполнена"}

func (a *Agent) goalsReply(ctx context.Context, req Request) Reply {
	a.step(req, "tool", toolLabel("goals_get"))
	goals, err := a.d.Goals(ctx, req.User)
	if err != nil {
		return Reply{Text: "Не получилось загрузить цели. Попробуй обновить страницу."}
	}
	if len(goals) == 0 {
		return Reply{Text: "Цели стажировки пока не поставлены. На первой неделе их согласует с тобой руководитель вместе с критериями успеха — можешь сам предложить встречу.",
			Answered: true, Suggestions: []string{"Кто мой руководитель?"}}
	}
	var b strings.Builder
	b.WriteString("Твои цели на стажировку и критерии успеха:")
	for _, g := range goals {
		fmt.Fprintf(&b, "\n- **%s** — %s. Срок: %s, %s, прогресс %d%%.", g.Title, g.Metric, ruDate(g.Due), goalStatusRu[g.Status], g.Progress)
	}
	b.WriteString("\n\nПрогресс отмечай на странице «Мой план» — руководитель видит его без отдельных отчётов.")
	return Reply{Text: b.String(), Answered: true, Suggestions: []string{"Открыть мой план"}}
}

func (a *Agent) whoReply(ctx context.Context, req Request) Reply {
	a.step(req, "tool", toolLabel("directory_whoami"))
	who, err := a.d.Corp.Call(ctx, req.Token, "directory_whoami", nil)
	if err != nil {
		return Reply{Text: unavailableText("справочника")}
	}
	var b strings.Builder
	team, _ := who["team"].(map[string]any)
	fmt.Fprintf(&b, "Твоя команда — **%v** (кластер %v), чат %v.\n", team["name"], team["cluster"], team["chat"])
	if m, ok := who["mentor"].(map[string]any); ok {
		fmt.Fprintf(&b, "- Наставник: %v (%v) — задачи, ревью, рабочие вопросы.\n", m["name"], m["handle"])
	}
	if l, ok := who["lead"].(map[string]any); ok {
		fmt.Fprintf(&b, "- Руководитель: %v (%v) — цели, приоритеты, согласования.\n", l["name"], l["handle"])
	}
	fmt.Fprintf(&b, "- Куратор стажировки: %v — методички, трекер прогресса, экватор.", team["curator"])
	return Reply{Text: b.String(), Answered: true, Suggestions: []string{"Расскажи про ритуалы команды"}}
}

func (a *Agent) kbAnswer(ctx context.Context, req Request) Reply {
	a.step(req, "tool", toolLabel("kb_search"))
	hits := a.d.KB.Search(req.Text, req.User, 4)
	if len(hits) == 0 || hits[0].Score < minScore || hits[0].Coverage < minCoverage {
		return Reply{Text: "В базе знаний команды я не нашёл надёжного ответа и не хочу гадать. Могу передать вопрос наставнику — он ответит, а ответ сохранится в базе для следующих стажёров.",
			Proposal: prepareMentorQuestion(req.Text), Escalated: true}
	}
	top := hits[0]
	var b strings.Builder
	fmt.Fprintf(&b, "**%s**\n%s", top.Section, top.Text)
	used := []knowledge.Hit{top}
	for _, h := range hits[1:] {
		if h.DocID == top.DocID && h.Score >= top.Score*0.5 && len(used) < 2 {
			fmt.Fprintf(&b, "\n\n**%s**\n%s", h.Section, h.Text)
			used = append(used, h)
		}
	}
	if top.Kind == "faq" {
		b.WriteString("\n\n_Это ответ наставника команды на похожий вопрос._")
	}
	return Reply{Text: b.String(), Sources: sourcesOf(used), Answered: true}
}
