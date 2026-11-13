package quiz

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuiltinAndGrade(t *testing.T) {
	qs, err := LoadBuiltin()
	if err != nil || len(qs) < 4 {
		t.Fatalf("builtin: %v %d", err, len(qs))
	}
	q := qs[0]
	pub := q.Public()
	b, _ := json.Marshal(pub)
	if strings.Contains(string(b), "correct") || len(pub.Questions) != len(q.Questions) {
		t.Fatalf("public leaks answers: %s", b)
	}
	right := make([]int, len(q.Questions))
	for i, x := range q.Questions {
		right[i] = x.Correct
	}
	a, rev, err := q.Grade(right)
	if err != nil || !a.Passed || a.Percent != 100 || !rev[0].Right {
		t.Fatalf("grade: %+v %v", a, err)
	}
	wrong := make([]int, len(q.Questions))
	for i := range wrong {
		wrong[i] = -1
	}
	if a, _, _ := q.Grade(wrong); a.Passed || a.Correct != 0 {
		t.Fatal("all skipped")
	}
	if _, _, err := q.Grade([]int{0}); err == nil {
		t.Fatal("answers count")
	}
	empty := (&Quiz{ID: "x", Title: "Тест", PassPercent: 50, Questions: []Question{{Text: "?", Options: []string{"a", "b"}}}}).Public()
	if empty.DocIDs == nil {
		t.Fatal("nil docs")
	}
}

func TestValidate(t *testing.T) {
	ok := Question{Text: "Вопрос", Options: []string{"a", "b"}, Correct: 1}
	cases := map[string]Quiz{
		"title":   {Title: "x", PassPercent: 50, Questions: []Question{ok}},
		"none":    {Title: "Тест", PassPercent: 50},
		"pass":    {Title: "Тест", PassPercent: 0, Questions: []Question{ok}},
		"text":    {Title: "Тест", PassPercent: 50, Questions: []Question{{Options: []string{"a", "b"}}}},
		"options": {Title: "Тест", PassPercent: 50, Questions: []Question{{Text: "?", Options: []string{"a"}}}},
		"empty":   {Title: "Тест", PassPercent: 50, Questions: []Question{{Text: "?", Options: []string{"a", " "}}}},
		"correct": {Title: "Тест", PassPercent: 50, Questions: []Question{{Text: "?", Options: []string{"a", "b"}, Correct: 2}}},
	}
	for name, q := range cases {
		if err := q.Validate(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	good := Quiz{Title: "Тест", PassPercent: 50, Questions: []Question{ok}}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
}
