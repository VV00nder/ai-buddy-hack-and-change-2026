// Package domain содержит общие типы AI-buddy.
package domain

import "time"

// Role — роль пользователя в процессе адаптации.
type Role string

const (
	RoleIntern Role = "intern"
	RoleMentor Role = "mentor"
	RoleLead   Role = "lead"
	RoleHRBP   Role = "hrbp"
	// RoleEngineer — сотрудник без управленческой роли (например, дежурный DevOps).
	RoleEngineer Role = "engineer"
)

// User — профиль сотрудника из корпоративного справочника (синтетика).
type User struct {
	ID        string    `json:"id"`
	Login     string    `json:"login"`
	Name      string    `json:"name"`
	Role      Role      `json:"role"`
	Team      string    `json:"team"`
	TeamName  string    `json:"teamName"`
	Track     string    `json:"track,omitempty"`
	MentorID  string    `json:"mentorId,omitempty"`
	LeadID    string    `json:"leadId,omitempty"`
	StartDate time.Time `json:"startDate"`
	Email     string    `json:"email,omitempty"`
	// Registered — учётная запись создана регистрацией (не тестовая).
	Registered bool `json:"registered,omitempty"`
}

// StepStatus — статус шага плана адаптации.
type StepStatus string

const (
	StepTodo       StepStatus = "todo"
	StepInProgress StepStatus = "in_progress"
	StepDone       StepStatus = "done"
	StepBlocked    StepStatus = "blocked"
)

// ValidStepStatus проверяет значение статуса.
func ValidStepStatus(s StepStatus) bool {
	switch s {
	case StepTodo, StepInProgress, StepDone, StepBlocked:
		return true
	}
	return false
}

// StepState — состояние шага плана конкретного стажёра.
type StepState struct {
	UserID    string     `json:"userId"`
	StepID    string     `json:"stepId"`
	Status    StepStatus `json:"status"`
	Note      string     `json:"note,omitempty"`
	UpdatedAt time.Time  `json:"updatedAt"`
	UpdatedBy string     `json:"updatedBy,omitempty"`
}

// MessageStatus — жизненный цикл ответа ассистента.
type MessageStatus string

const (
	MsgPending MessageStatus = "pending"
	MsgDone    MessageStatus = "done"
	MsgError   MessageStatus = "error"
)

// Source — ссылка на документ базы знаний, использованный в ответе.
type Source struct {
	DocID   string `json:"docId"`
	Title   string `json:"title"`
	Section string `json:"section,omitempty"`
}

// TraceItem — шаг работы агента, который видит пользователь (статус ожидания).
type TraceItem struct {
	Kind  string    `json:"kind"` // thinking | tool | fallback
	Label string    `json:"label"`
	At    time.Time `json:"at"`
}

// Message — сообщение в диалоге стажёра с AI-buddy.
type Message struct {
	ID          string        `json:"id"`
	UserID      string        `json:"userId"`
	Author      string        `json:"author"` // user | assistant
	Text        string        `json:"text"`
	Status      MessageStatus `json:"status"`
	Sources     []Source      `json:"sources,omitempty"`
	ActionID    string        `json:"actionId,omitempty"`
	Suggestions []string      `json:"suggestions,omitempty"`
	// QuizID — тест, который ассистент предлагает пройти (кнопка в сообщении).
	QuizID   string      `json:"quizId,omitempty"`
	Trace    []TraceItem `json:"trace,omitempty"`
	Mode     string      `json:"mode,omitempty"` // llm | offline
	Feedback *bool       `json:"feedback,omitempty"`
	// Context — служебное состояние диалога (например, ассистент ждёт дату отгула).
	Context   map[string]string `json:"context,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// ActionKind — тип действия через корпоративный сервис.
type ActionKind string

const (
	ActionAbsence        ActionKind = "absence"
	ActionAccess         ActionKind = "access"
	ActionMentorQuestion ActionKind = "mentor_question"
)

// ActionStatus — статус действия.
type ActionStatus string

const (
	ActionProposed  ActionStatus = "proposed"  // черновик, ждёт подтверждения стажёра
	ActionSubmitted ActionStatus = "submitted" // отправлено в сервис, ждёт решения
	ActionApproved  ActionStatus = "approved"
	ActionRejected  ActionStatus = "rejected"
	ActionCancelled ActionStatus = "cancelled"
	ActionFailed    ActionStatus = "failed"
)

// Action — действие, которое AI-buddy готовит и выполняет через корпоративный сервис (MCP).
type Action struct {
	ID         string         `json:"id"`
	UserID     string         `json:"userId"`
	Kind       ActionKind     `json:"kind"`
	Tool       string         `json:"tool"`
	Title      string         `json:"title"`
	Args       map[string]any `json:"args"`
	Preview    map[string]any `json:"preview,omitempty"` // результат проверки (dry-run)
	Warnings   []string       `json:"warnings,omitempty"`
	Status     ActionStatus   `json:"status"`
	ExternalID string         `json:"externalId,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
	Error      string         `json:"error,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
}

// Question — вопрос стажёра, переданный наставнику.
type Question struct {
	ID         string     `json:"id"`
	InternID   string     `json:"internId"`
	MentorID   string     `json:"mentorId"`
	Text       string     `json:"text"`
	Context    string     `json:"context,omitempty"`
	Answer     string     `json:"answer,omitempty"`
	AddToFAQ   bool       `json:"addToFaq"`
	Status     string     `json:"status"` // open | answered
	CreatedAt  time.Time  `json:"createdAt"`
	AnsweredAt *time.Time `json:"answeredAt,omitempty"`
}

// Notification — уведомление в кабинете.
type Notification struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	Link      string    `json:"link,omitempty"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"createdAt"`
}

// FAQEntry — ответ наставника, сохранённый для команды.
type FAQEntry struct {
	ID        string    `json:"id"`
	Team      string    `json:"team"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	AuthorID  string    `json:"authorId"`
	CreatedAt time.Time `json:"createdAt"`
}

// GoalStatus — статус цели стажировки.
type GoalStatus string

const (
	GoalAgreed     GoalStatus = "agreed"
	GoalInProgress GoalStatus = "in_progress"
	GoalDone       GoalStatus = "done"
)

// Goal — измеримая цель на период стажировки (трекер прогресса из чек-листа).
type Goal struct {
	ID        string     `json:"id"`
	InternID  string     `json:"internId"`
	Title     string     `json:"title"`
	Metric    string     `json:"metric"` // критерий успеха
	Due       string     `json:"due"`    // ГГГГ-ММ-ДД
	Status    GoalStatus `json:"status"`
	Progress  int        `json:"progress"` // 0..100
	Note      string     `json:"note,omitempty"`
	CreatedBy string     `json:"createdBy"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// Event — продуктовое событие (совместимо по форме с clickstream).
type Event struct {
	ID     string         `json:"id"`
	UserID string         `json:"userId"`
	Type   string         `json:"type"`
	Props  map[string]any `json:"props,omitempty"`
	At     time.Time      `json:"at"`
}
