package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"aibuddy/internal/agent"
	"aibuddy/internal/domain"
	"aibuddy/internal/quiz"
)

// Тесты на проверку усвоенного материала: после темы (инструкции) или
// блока тем (этапа плана). Тест закрывает шаг плана, к которому привязан.

// QuizInfo — тест в списке с результатом пользователя.
type QuizInfo struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	DocIDs      []string `json:"docIds"`
	Questions   int      `json:"questions"`
	PassPercent int      `json:"passPercent"`
	Builtin     bool     `json:"builtin"`
	Editable    bool     `json:"editable"`
	// Результат текущего пользователя: лучший процент и сдан ли тест.
	Attempts int  `json:"attempts"`
	Best     int  `json:"best"`
	Passed   bool `json:"passed"`
}

func quizVisible(q *quiz.Quiz, u domain.User) bool {
	return q.Team == "all" || q.Team == "" || sameTeam(u, q.Team)
}

func (a *App) allQuizzes(ctx context.Context) ([]*quiz.Quiz, error) {
	out := append([]*quiz.Quiz{}, a.builtinQuiz...)
	custom, err := listRec[*quiz.Quiz](ctx, a.store, recQuiz)
	if err != nil {
		return nil, err
	}
	return append(out, custom...), nil
}

func (a *App) quizByID(ctx context.Context, u domain.User, id string) (*quiz.Quiz, error) {
	all, err := a.allQuizzes(ctx)
	if err != nil {
		return nil, err
	}
	for _, q := range all {
		if q.ID == id && quizVisible(q, u) {
			return q, nil
		}
	}
	return nil, ErrNotFound
}

func (a *App) attemptsOf(ctx context.Context, userID string) ([]quiz.Attempt, error) {
	all, err := listRec[quiz.Attempt](ctx, a.store, recQuizAttempt)
	if err != nil {
		return nil, err
	}
	out := []quiz.Attempt{}
	for _, at := range all {
		if at.UserID == userID {
			out = append(out, at)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

// Quizzes — доступные тесты с результатами пользователя.
func (a *App) Quizzes(ctx context.Context, u domain.User) ([]QuizInfo, error) {
	all, err := a.allQuizzes(ctx)
	if err != nil {
		return nil, err
	}
	atts, err := a.attemptsOf(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	editor := a.canEditKB(ctx, u)
	out := []QuizInfo{}
	for _, q := range all {
		if !quizVisible(q, u) {
			continue
		}
		info := QuizInfo{ID: q.ID, Title: q.Title, Description: q.Description, DocIDs: q.DocIDs, Questions: len(q.Questions),
			PassPercent: q.PassPercent, Builtin: q.Builtin, Editable: editor && !q.Builtin && sameTeam(u, q.Team)}
		if info.DocIDs == nil {
			info.DocIDs = []string{}
		}
		for _, at := range atts {
			if at.QuizID != q.ID {
				continue
			}
			info.Attempts++
			info.Best = max(info.Best, at.Percent)
			info.Passed = info.Passed || at.Passed
		}
		out = append(out, info)
	}
	return out, nil
}

// QuizToTake — тест без правильных ответов.
func (a *App) QuizToTake(ctx context.Context, u domain.User, id string) (quiz.Public, error) {
	q, err := a.quizByID(ctx, u, id)
	if err != nil {
		return quiz.Public{}, err
	}
	return q.Public(), nil
}

// QuizToEdit — тест целиком (с ответами) для редактора инструкций.
func (a *App) QuizToEdit(ctx context.Context, u domain.User, id string) (*quiz.Quiz, error) {
	if !a.canEditKB(ctx, u) {
		return nil, ErrForbidden
	}
	return a.quizByID(ctx, u, id)
}

// QuizResult — итог попытки с разбором ответов.
type QuizResult struct {
	Attempt quiz.Attempt  `json:"attempt"`
	Review  []quiz.Review `json:"review"`
	// StepsDone — шаги плана, закрытые сдачей теста.
	StepsDone []string `json:"stepsDone"`
}

// SubmitQuiz проверяет ответы, сохраняет попытку и при сдаче закрывает шаги плана.
func (a *App) SubmitQuiz(ctx context.Context, u domain.User, id string, answers []int) (QuizResult, error) {
	q, err := a.quizByID(ctx, u, id)
	if err != nil {
		return QuizResult{}, err
	}
	at, rev, err := q.Grade(answers)
	if err != nil {
		return QuizResult{}, invalid("%s", err.Error())
	}
	at.ID, at.UserID, at.At = newID("qa"), u.ID, a.stamp() // монотонное время — порядок попыток стабилен
	if err := putRec(ctx, a.store, recQuizAttempt, at.ID, at); err != nil {
		return QuizResult{}, err
	}
	_ = a.store.DeleteRecord(ctx, recQuizLock, u.ID) // тест завершён — чат и инструкции снова доступны
	res := QuizResult{Attempt: at, Review: a.titled(u, quiz.HideAnswers(rev)), StepsDone: []string{}}
	typ := "quiz_failed"
	if at.Passed {
		typ = "quiz_passed"
	}
	a.Track(ctx, u.ID, typ, map[string]any{"quiz": q.ID, "percent": at.Percent})
	if u.Role != domain.RoleIntern {
		return res, nil
	}
	if at.Passed {
		res.StepsDone = a.completeQuizSteps(ctx, u, q)
	}
	if u.MentorID != "" {
		verdict := "сдал(а)"
		if !at.Passed {
			verdict = "не сдал(а)"
		}
		a.notify(ctx, u.MentorID, "quiz", fmt.Sprintf("%s %s тест «%s»: %d%% (%d из %d)", u.Name, verdict, q.Title, at.Percent, at.Correct, at.Total), "/team")
	}
	return res, nil
}

func (a *App) completeQuizSteps(ctx context.Context, intern domain.User, q *quiz.Quiz) []string {
	tpl, _ := a.templateFor(ctx, intern)
	states, _ := a.store.StepStates(ctx, intern.ID)
	done := map[string]bool{}
	for _, s := range states {
		done[s.StepID] = s.Status == domain.StepDone
	}
	out := []string{}
	for _, id := range tpl.StepsWithQuiz(q.ID) {
		if done[id] {
			continue
		}
		_ = a.store.SetStepState(ctx, domain.StepState{UserID: intern.ID, StepID: id, Status: domain.StepDone,
			Note: "Тест сдан", UpdatedAt: a.now(), UpdatedBy: "ai-buddy"})
		a.Track(ctx, intern.ID, "step_completed", map[string]any{"step": id, "by": "quiz"})
		out = append(out, id)
	}
	if len(out) > 0 {
		a.publishPlan(ctx, intern)
	}
	return out
}

// AttemptView — попытка с разбором по вопросам (для наставника, руководителя и самого стажёра).
type AttemptView struct {
	quiz.Attempt
	Review []quiz.Review `json:"review"`
}

// withReview восстанавливает разбор попытки по тексту теста.
func (a *App) withReview(ctx context.Context, intern domain.User, atts []quiz.Attempt) []AttemptView {
	out := make([]AttemptView, 0, len(atts))
	cache := map[string]*quiz.Quiz{}
	for _, at := range atts {
		v := AttemptView{Attempt: at, Review: []quiz.Review{}}
		q, ok := cache[at.QuizID]
		if !ok {
			q, _ = a.quizByID(ctx, intern, at.QuizID)
			cache[at.QuizID] = q
		}
		if q != nil && len(q.Questions) == len(at.Answers) {
			_, rev, _ := q.Grade(at.Answers)
			v.Review = a.titled(intern, rev)
		}
		out = append(out, v)
	}
	return out
}

// QuizResults — попытки стажёра с ответами на каждый вопрос (для самого стажёра,
// наставника, руководителя, HR BP).
func (a *App) QuizResults(ctx context.Context, viewer domain.User, internID string) ([]AttemptView, error) {
	if internID == "" {
		internID = viewer.ID
	}
	intern, ok := a.userByID(ctx, internID)
	if !ok {
		return nil, ErrNotFound
	}
	if !canViewIntern(viewer, intern) {
		return nil, ErrForbidden
	}
	atts, err := a.attemptsOf(ctx, internID)
	if err != nil {
		return nil, err
	}
	views := a.withReview(ctx, intern, atts)
	if viewer.ID == intern.ID { // себе — без готовых ответов, со ссылками на инструкции
		for i := range views {
			views[i].Review = quiz.HideAnswers(views[i].Review)
		}
	}
	return views, nil
}

// InternQuizResults — результаты одного стажёра в сводке команды.
type InternQuizResults struct {
	Intern   domain.User   `json:"intern"`
	Attempts []AttemptView `json:"attempts"`
}

// TeamQuizResults — результаты тестов всех стажёров, которых видит пользователь.
func (a *App) TeamQuizResults(ctx context.Context, viewer domain.User) ([]InternQuizResults, error) {
	if viewer.Role == domain.RoleIntern {
		return nil, ErrForbidden
	}
	users, err := a.DemoUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := []InternQuizResults{}
	for _, u := range users {
		if u.Role != domain.RoleIntern || !canViewIntern(viewer, u) {
			continue
		}
		atts, err := a.attemptsOf(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, InternQuizResults{Intern: u, Attempts: a.withReview(ctx, u, atts)})
	}
	return out, nil
}

// SaveQuiz создаёт или изменяет тест команды.
func (a *App) SaveQuiz(ctx context.Context, u domain.User, in quiz.Quiz) (*quiz.Quiz, error) {
	if !a.canEditKB(ctx, u) {
		return nil, ErrForbidden
	}
	team := contentTeam(u)
	if in.ID == "" {
		in.ID = newID("quiz")
	} else {
		cur, err := a.quizByID(ctx, u, in.ID)
		if err != nil {
			return nil, err
		}
		if cur.Builtin {
			return nil, invalid("Встроенный тест нельзя изменить — создайте новый")
		}
		if !sameTeam(u, cur.Team) {
			return nil, ErrForbidden
		}
		team = cur.Team
	}
	q := &quiz.Quiz{ID: in.ID, Title: strings.TrimSpace(in.Title), Description: strings.TrimSpace(in.Description), Team: team,
		PassPercent: in.PassPercent, Questions: in.Questions, DocIDs: []string{}, UpdatedBy: u.Name, UpdatedAt: a.now().Format(time.RFC3339)}
	if q.PassPercent == 0 {
		q.PassPercent = 80
	}
	for _, d := range in.DocIDs {
		if _, ok := a.kb.Get(d, u); !ok {
			return nil, invalid("Инструкция «%s» не найдена", d)
		}
		q.DocIDs = append(q.DocIDs, d)
	}
	if err := q.Validate(); err != nil {
		return nil, invalid("%s", err.Error())
	}
	if err := putRec(ctx, a.store, recQuiz, q.ID, q); err != nil {
		return nil, err
	}
	a.Track(ctx, u.ID, "quiz_saved", map[string]any{"quiz": q.ID, "questions": len(q.Questions)})
	return q, nil
}

// DeleteQuiz удаляет тест команды.
func (a *App) DeleteQuiz(ctx context.Context, u domain.User, id string) error {
	q, err := a.QuizToEdit(ctx, u, id)
	if err != nil {
		return err
	}
	if q.Builtin {
		return invalid("Встроенный тест нельзя удалить")
	}
	if !sameTeam(u, q.Team) {
		return ErrForbidden
	}
	return a.store.DeleteRecord(ctx, recQuiz, id)
}

// QuizRefs — тесты для агента.
func (a *App) QuizRefs(ctx context.Context, u domain.User) ([]agent.QuizRef, error) {
	qs, err := a.Quizzes(ctx, u)
	if err != nil {
		return nil, err
	}
	out := make([]agent.QuizRef, 0, len(qs))
	for _, q := range qs {
		out = append(out, agent.QuizRef{ID: q.ID, Title: q.Title, DocIDs: q.DocIDs, Passed: q.Passed, Best: q.Best})
	}
	return out, nil
}

// titled подставляет названия инструкций в разбор.
func (a *App) titled(u domain.User, rev []quiz.Review) []quiz.Review {
	for i := range rev {
		if d, ok := a.kb.Get(rev[i].DocID, u); ok {
			rev[i].DocTitle = d.Title
		} else {
			rev[i].DocID = ""
		}
	}
	return rev
}

// QuizLock — тест в процессе: пока он идёт, инструкции и чат закрыты.
type QuizLock struct {
	QuizID string    `json:"quizId"`
	Title  string    `json:"title"`
	Until  time.Time `json:"until"`
}

// quizLockTTL — сколько держится блокировка, если тест бросили.
const quizLockTTL = 30 * time.Minute

// StartQuiz начинает тест: возвращает вопросы и закрывает инструкции и чат до сдачи.
func (a *App) StartQuiz(ctx context.Context, u domain.User, id string) (quiz.Public, error) {
	q, err := a.quizByID(ctx, u, id)
	if err != nil {
		return quiz.Public{}, err
	}
	if err := putRec(ctx, a.store, recQuizLock, u.ID, QuizLock{QuizID: q.ID, Title: q.Title, Until: a.now().Add(quizLockTTL)}); err != nil {
		return quiz.Public{}, err
	}
	a.Track(ctx, u.ID, "quiz_started", map[string]any{"quiz": q.ID})
	return q.Public(), nil
}

// CancelQuiz прерывает тест без попытки.
func (a *App) CancelQuiz(ctx context.Context, u domain.User) error {
	return a.store.DeleteRecord(ctx, recQuizLock, u.ID)
}

// ActiveQuiz — идущий тест пользователя, если есть.
func (a *App) ActiveQuiz(ctx context.Context, u domain.User) (QuizLock, bool) {
	l, err := getRec[QuizLock](ctx, a.store, recQuizLock, u.ID)
	if err != nil || !a.now().Before(l.Until) {
		return QuizLock{}, false
	}
	return l, true
}
