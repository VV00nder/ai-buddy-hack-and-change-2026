package agent

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"aibuddy/internal/domain"
	"aibuddy/internal/knowledge"
	"aibuddy/internal/onboarding"
)

// Отметка шагов плана из чата и тесты на проверку знаний.

var reStepDone = regexp.MustCompile(`(^|\s)(отметь|отметить|отмечай|закрой шаг|я сделал|я сделала|сделал шаг|сделала шаг|я выполнил|я выполнила|выполнил шаг|выполнила шаг|шаг выполнен|уже сделал|уже сделала|я закончил|я закончила|я настроил|я настроила|я изучил|я изучила|я прочитал|я прочитала|я завел|я завела|я вступил|я вступила|я познакомился|я познакомилась|готово:)`)

var reQuiz = regexp.MustCompile(`(^|[\s«"(])(тест|теста|тесты|тестом|тесту|тестик|квиз|проверь меня|проверь мои знания|проверить знания|проверка знаний|проверить себя|проверь себя)($|[\s?.!,»")])`)

// isStepDoneIntent — стажёр сообщает, что сделал шаг плана.
func isStepDoneIntent(t string) bool { return reStepDone.MatchString(t) }

// isQuizIntent — просьба проверить знания.
func isQuizIntent(t string) bool {
	return reQuiz.MatchString(t) && !strings.Contains(t, "тестов")
}

// matchStep — шаг плана, о котором говорит текст: по совпадению слов названия.
func matchStep(p onboarding.Plan, text string) *onboarding.StepView {
	t := strings.ToLower(text)
	if strings.Contains(t, "следующ") && p.Next != nil {
		return p.Next
	}
	words := map[string]bool{}
	for _, w := range knowledge.Tokenize(text) {
		words[w] = true
	}
	var best *onboarding.StepView
	bestScore := 0.0
	for _, per := range p.Periods {
		for i := range per.Steps {
			s := per.Steps[i]
			title := knowledge.Tokenize(s.Title)
			if len(title) == 0 {
				continue
			}
			hit := 0
			for _, w := range title {
				if words[w] {
					hit++
				}
			}
			score := float64(hit) / float64(len(title))
			if s.Status != domain.StepDone {
				score += 0.05 // при равенстве — невыполненный шаг
			}
			if hit > 0 && score > bestScore {
				bestScore, best = score, &s
			}
		}
	}
	if bestScore < 0.4 {
		return nil
	}
	return best
}

func (a *Agent) stepDone(ctx context.Context, req Request) Reply {
	if a.d.Plan == nil || a.d.SetStep == nil || req.User.Role != domain.RoleIntern {
		return Reply{Text: "Отмечать шаги можно только в своём плане адаптации.", Answered: true}
	}
	a.step(req, "tool", "Смотрю план адаптации")
	p, err := a.d.Plan(ctx, req.User)
	if err != nil {
		return Reply{Text: "План сейчас недоступен — попробуй через минуту.", Answered: true}
	}
	s := matchStep(p, req.Text)
	if s == nil {
		r := Reply{Text: "Не понял, какой шаг отметить. Выбери шаг или напиши его название.", Answered: true}
		for _, per := range p.Periods {
			if per.State == "future" {
				continue
			}
			for _, st := range per.Steps {
				if st.Owner == "intern" && st.Status != domain.StepDone && st.Quiz == "" && len(r.Suggestions) < 3 {
					r.Suggestions = append(r.Suggestions, "Отметь выполненным: "+st.Title)
				}
			}
		}
		return r
	}
	switch {
	case s.Status == domain.StepDone:
		return Reply{Text: fmt.Sprintf("Шаг **%s** уже отмечен выполненным.", s.Title), Answered: true, Suggestions: []string{"Что мне делать дальше?"}}
	case s.Owner != "intern":
		return Reply{Text: fmt.Sprintf("Шаг **%s** отмечает наставник. Напомни ему на ближайшей встрече.", s.Title), Answered: true}
	case s.Quiz != "":
		return Reply{Text: fmt.Sprintf("Шаг **%s** закроется сам, когда ты сдашь тест. Готов попробовать?", s.Title), Answered: true, QuizID: s.Quiz}
	}
	a.step(req, "tool", "Отмечаю шаг в плане")
	np, err := a.d.SetStep(ctx, req.User, s.ID, domain.StepDone)
	if err != nil {
		return Reply{Text: "Не получилось отметить шаг: " + err.Error(), Answered: true}
	}
	text := fmt.Sprintf("Отметил шаг **%s** выполненным. Прогресс плана: %d%% (%d из %d).", s.Title, np.Percent, np.Done, np.Total)
	if np.Next != nil {
		text += fmt.Sprintf("\n\nСледующий шаг: **%s**.", np.Next.Title)
	}
	r := Reply{Text: text, Answered: true, Suggestions: []string{"Что мне делать дальше?"}}
	if q := a.quizForDocs(ctx, req.User, s.Docs); q != nil && !q.Passed {
		r.Text += fmt.Sprintf("\n\nПо этой теме есть тест «%s» — можно проверить себя.", q.Title)
		r.QuizID = q.ID
	}
	return r
}

func (a *Agent) quizForDocs(ctx context.Context, u domain.User, docs []string) *QuizRef {
	if a.d.Quizzes == nil || len(docs) == 0 {
		return nil
	}
	qs, err := a.d.Quizzes(ctx, u)
	if err != nil {
		return nil
	}
	for i := range qs {
		for _, d := range qs[i].DocIDs {
			if slices.Contains(docs, d) {
				return &qs[i]
			}
		}
	}
	return nil
}

func (a *Agent) quizReply(ctx context.Context, req Request) Reply {
	if a.d.Quizzes == nil {
		return a.kbAnswer(ctx, req)
	}
	a.step(req, "tool", "Подбираю тест")
	qs, err := a.d.Quizzes(ctx, req.User)
	if err != nil || len(qs) == 0 {
		return Reply{Text: "Тестов пока нет — их добавляют руководитель и наставники во вкладке «Тесты».", Answered: true}
	}
	// 1. тест по теме вопроса
	var docs []string
	for _, h := range a.d.KB.Search(req.Text, req.User, 3) {
		if h.Coverage >= minCoverage {
			docs = append(docs, h.DocID)
		}
	}
	pick := a.quizForDocs(ctx, req.User, docs)
	// 2. тест из текущего этапа плана
	if pick == nil && a.d.Plan != nil {
		if p, err := a.d.Plan(ctx, req.User); err == nil {
			for _, per := range p.Periods {
				if per.State == "future" {
					continue
				}
				for _, s := range per.Steps {
					if s.Quiz != "" && s.Status != domain.StepDone {
						for i := range qs {
							if qs[i].ID == s.Quiz && pick == nil {
								pick = &qs[i]
							}
						}
					}
				}
			}
		}
	}
	// 3. первый несданный
	if pick == nil {
		for i := range qs {
			if !qs[i].Passed {
				pick = &qs[i]
				break
			}
		}
	}
	if pick == nil {
		return Reply{Text: "Все доступные тесты уже сданы. Отличная работа!", Answered: true, Suggestions: []string{"Что мне делать дальше?"}}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Предлагаю тест **«%s»**. Ответы проверяются сразу, после сдачи увидишь разбор.", pick.Title)
	var others []string
	for _, q := range qs {
		if q.ID != pick.ID {
			mark := ""
			if q.Passed {
				mark = " — сдан"
			}
			others = append(others, fmt.Sprintf("- %s%s", q.Title, mark))
		}
	}
	if len(others) > 0 {
		b.WriteString("\n\nДругие тесты:\n" + strings.Join(others, "\n"))
	}
	return Reply{Text: b.String(), Answered: true, QuizID: pick.ID}
}
