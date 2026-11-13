// Package quiz — тесты на проверку усвоенного материала: после темы
// (инструкции) или блока тем (этапа плана). Правильные ответы не уходят
// в интерфейс до сдачи — проверка только на сервере.
package quiz

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

//go:embed builtin.json
var builtinJSON []byte

// Question — вопрос с вариантами ответа.
type Question struct {
	Text    string   `json:"text"`
	Options []string `json:"options"`
	Correct int      `json:"correct"` // индекс правильного варианта
	Explain string   `json:"explain,omitempty"`
}

// Quiz — тест по теме или блоку тем.
type Quiz struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	DocIDs      []string   `json:"docIds"` // инструкции, которые проверяет тест
	Team        string     `json:"team"`   // all — для всех команд
	PassPercent int        `json:"passPercent"`
	Questions   []Question `json:"questions"`
	Builtin     bool       `json:"builtin"`
	UpdatedBy   string     `json:"updatedBy,omitempty"`
	UpdatedAt   string     `json:"updatedAt,omitempty"`
}

// PublicQuestion — вопрос без правильного ответа.
type PublicQuestion struct {
	Text    string   `json:"text"`
	Options []string `json:"options"`
}

// Public — тест для прохождения: без правильных ответов и пояснений.
type Public struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Description string           `json:"description,omitempty"`
	DocIDs      []string         `json:"docIds"`
	PassPercent int              `json:"passPercent"`
	Questions   []PublicQuestion `json:"questions"`
}

// Attempt — попытка прохождения.
type Attempt struct {
	ID      string    `json:"id"`
	QuizID  string    `json:"quizId"`
	Title   string    `json:"title"`
	UserID  string    `json:"userId"`
	Answers []int     `json:"answers"`
	Correct int       `json:"correct"`
	Total   int       `json:"total"`
	Percent int       `json:"percent"`
	Passed  bool      `json:"passed"`
	At      time.Time `json:"at"`
}

// Review — разбор ответа после сдачи.
type Review struct {
	Text    string   `json:"text"`
	Options []string `json:"options"`
	Chosen  int      `json:"chosen"`
	Correct int      `json:"correct"`
	Right   bool     `json:"right"`
	Explain string   `json:"explain,omitempty"`
}

// LoadBuiltin — встроенные тесты по инструкциям прототипа.
func LoadBuiltin() ([]*Quiz, error) {
	var out []*Quiz
	if err := json.Unmarshal(builtinJSON, &out); err != nil {
		return nil, err
	}
	for _, q := range out {
		q.Builtin = true
		if err := q.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", q.ID, err)
		}
	}
	return out, nil
}

// Validate проверяет тест перед сохранением.
func (q *Quiz) Validate() error {
	if n := utf8.RuneCountInString(strings.TrimSpace(q.Title)); n < 3 || n > 150 {
		return fmt.Errorf("Название теста — от 3 до 150 символов")
	}
	if len(q.Questions) == 0 || len(q.Questions) > 30 {
		return fmt.Errorf("В тесте должно быть от 1 до 30 вопросов")
	}
	if q.PassPercent < 1 || q.PassPercent > 100 {
		return fmt.Errorf("Проходной балл — от 1 до 100%%")
	}
	for i, qq := range q.Questions {
		if strings.TrimSpace(qq.Text) == "" || utf8.RuneCountInString(qq.Text) > 500 {
			return fmt.Errorf("Вопрос %d: текст от 1 до 500 символов", i+1)
		}
		if len(qq.Options) < 2 || len(qq.Options) > 6 {
			return fmt.Errorf("Вопрос %d: нужно от 2 до 6 вариантов ответа", i+1)
		}
		for _, o := range qq.Options {
			if strings.TrimSpace(o) == "" || utf8.RuneCountInString(o) > 300 {
				return fmt.Errorf("Вопрос %d: пустой или слишком длинный вариант ответа", i+1)
			}
		}
		if qq.Correct < 0 || qq.Correct >= len(qq.Options) {
			return fmt.Errorf("Вопрос %d: отметьте правильный вариант", i+1)
		}
	}
	return nil
}

// Public возвращает тест без ответов.
func (q *Quiz) Public() Public {
	p := Public{ID: q.ID, Title: q.Title, Description: q.Description, DocIDs: q.DocIDs, PassPercent: q.PassPercent}
	if p.DocIDs == nil {
		p.DocIDs = []string{}
	}
	for _, qq := range q.Questions {
		p.Questions = append(p.Questions, PublicQuestion{Text: qq.Text, Options: qq.Options})
	}
	return p
}

// Grade проверяет ответы. Число ответов должно совпадать с числом вопросов
// (-1 — вопрос пропущен).
func (q *Quiz) Grade(answers []int) (Attempt, []Review, error) {
	if len(answers) != len(q.Questions) {
		return Attempt{}, nil, fmt.Errorf("Ответьте на все вопросы")
	}
	a := Attempt{QuizID: q.ID, Title: q.Title, Answers: answers, Total: len(q.Questions)}
	var rev []Review
	for i, qq := range q.Questions {
		right := answers[i] == qq.Correct
		if right {
			a.Correct++
		}
		rev = append(rev, Review{Text: qq.Text, Options: qq.Options, Chosen: answers[i], Correct: qq.Correct, Right: right, Explain: qq.Explain})
	}
	a.Percent = a.Correct * 100 / a.Total
	a.Passed = a.Percent >= q.PassPercent
	return a, rev, nil
}
