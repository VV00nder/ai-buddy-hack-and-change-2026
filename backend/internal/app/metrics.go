package app

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"aibuddy/internal/domain"
)

// Metrics — технические счётчики (экспорт в формате Prometheus для Grafana).
type Metrics struct {
	mu        sync.Mutex
	counters  map[string]int64
	latSum    float64
	latCount  int64
	latBucket [6]int64 // ≤1, ≤3, ≤5, ≤10, ≤30, +Inf секунд
}

var latBounds = [...]float64{1, 3, 5, 10, 30}

// NewMetrics создаёт счётчики.
func NewMetrics() *Metrics { return &Metrics{counters: map[string]int64{}} }

// Inc увеличивает счётчик событий.
func (m *Metrics) Inc(name string) {
	m.mu.Lock()
	m.counters[name]++
	m.mu.Unlock()
}

// ObserveAgent фиксирует время ответа агента.
func (m *Metrics) ObserveAgent(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := d.Seconds()
	m.latSum += s
	m.latCount++
	i := len(latBounds)
	for j, b := range latBounds {
		if s <= b {
			i = j
			break
		}
	}
	m.latBucket[i]++
}

// WritePrometheus пишет метрики в текстовом формате.
func (m *Metrics) WritePrometheus(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fmt.Fprintln(w, "# HELP buddy_events_total Продуктовые события AI-buddy")
	fmt.Fprintln(w, "# TYPE buddy_events_total counter")
	names := make([]string, 0, len(m.counters))
	for k := range m.counters {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Fprintf(w, "buddy_events_total{type=%q} %d\n", k, m.counters[k])
	}
	fmt.Fprintln(w, "# HELP buddy_agent_seconds Время ответа агента")
	fmt.Fprintln(w, "# TYPE buddy_agent_seconds histogram")
	var cum int64
	for i, b := range latBounds {
		cum += m.latBucket[i]
		fmt.Fprintf(w, "buddy_agent_seconds_bucket{le=\"%g\"} %d\n", b, cum)
	}
	cum += m.latBucket[len(latBounds)]
	fmt.Fprintf(w, "buddy_agent_seconds_bucket{le=\"+Inf\"} %d\n", cum)
	fmt.Fprintf(w, "buddy_agent_seconds_sum %g\nbuddy_agent_seconds_count %d\n", m.latSum, m.latCount)
}

// Insights — продуктовые метрики пользы для руководителя и HR BP.
type Insights struct {
	Questions          int       `json:"questions"`
	AnsweredByBuddy    int       `json:"answeredByBuddy"`
	SelfServeRate      int       `json:"selfServeRate"` // % вопросов без участия наставника
	Escalations        int       `json:"escalations"`
	MentorAnswers      int       `json:"mentorAnswers"`
	ActionsSubmitted   int       `json:"actionsSubmitted"`
	ActionsCompleted   int       `json:"actionsCompleted"`
	HelpfulRate        int       `json:"helpfulRate"` // % положительных оценок
	Feedbacks          int       `json:"feedbacks"`
	AvgAnswerSeconds   float64   `json:"avgAnswerSeconds"`
	StepsCompleted     int       `json:"stepsCompleted"`
	MentorMinutesSaved int       `json:"mentorMinutesSaved"`
	ByDay              []DayStat `json:"byDay"`
}

// DayStat — динамика по дням.
type DayStat struct {
	Day         string `json:"day"`
	Questions   int    `json:"questions"`
	Escalations int    `json:"escalations"`
}

// Оценка экономии времени наставника: рутинный вопрос ≈ 10 минут
// (переключение контекста + ответ), оформление заявки с помощью ≈ 15 минут.
const (
	minutesPerQuestion = 10
	minutesPerAction   = 15
)

// Insights считает метрики по событиям.
func (a *App) Insights(ctx context.Context, u domain.User) (Insights, error) {
	if u.Role != domain.RoleLead && u.Role != domain.RoleHRBP && u.Role != domain.RoleMentor && u.Role != domain.RoleAdmin {
		return Insights{}, ErrForbidden
	}
	evs, err := a.store.Events(ctx)
	if err != nil {
		return Insights{}, err
	}
	var in Insights
	var helpful int
	var latSum float64
	byDay := map[string]*DayStat{}
	for _, e := range evs {
		day := e.At.Format("2006-01-02")
		if byDay[day] == nil {
			byDay[day] = &DayStat{Day: day}
		}
		switch e.Type {
		case "question_answered":
			in.Questions++
			byDay[day].Questions++
			if e.Props["answered"] == true {
				in.AnsweredByBuddy++
			}
			if e.Props["escalated"] == true {
				in.Escalations++
				byDay[day].Escalations++
			}
			if s, ok := e.Props["seconds"].(float64); ok {
				latSum += s
			}
		case "mentor_answered":
			in.MentorAnswers++
		case "action_submitted":
			in.ActionsSubmitted++
		case "action_completed":
			in.ActionsCompleted++
		case "feedback":
			in.Feedbacks++
			if e.Props["helpful"] == true {
				helpful++
			}
		case "step_completed":
			in.StepsCompleted++
		}
	}
	if in.Questions > 0 {
		in.SelfServeRate = (in.Questions - in.Escalations) * 100 / in.Questions
		in.AvgAnswerSeconds = float64(int(latSum/float64(in.Questions)*10)) / 10
	}
	if in.Feedbacks > 0 {
		in.HelpfulRate = helpful * 100 / in.Feedbacks
	}
	in.MentorMinutesSaved = (in.Questions-in.Escalations)*minutesPerQuestion + in.ActionsSubmitted*minutesPerAction
	for _, d := range byDay {
		in.ByDay = append(in.ByDay, *d)
	}
	sort.Slice(in.ByDay, func(i, j int) bool { return in.ByDay[i].Day < in.ByDay[j].Day })
	return in, nil
}
