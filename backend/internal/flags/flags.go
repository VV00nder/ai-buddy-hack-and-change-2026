// Package flags — feature flags прототипа (в Авито — внутренний сервис
// feature flags; здесь значения из переменных окружения).
package flags

import (
	"os"
	"strconv"
)

// Flags — включённые возможности.
type Flags struct {
	AbsenceActions   bool `json:"absenceActions"`
	AccessActions    bool `json:"accessActions"`
	MentorEscalation bool `json:"mentorEscalation"`
	FAQFromAnswers   bool `json:"faqFromAnswers"`
	LLM              bool `json:"llm"`
	Proactive        bool `json:"proactive"`
}

// FromEnv читает флаги; по умолчанию всё включено.
func FromEnv(getenv func(string) string) Flags {
	if getenv == nil {
		getenv = os.Getenv
	}
	b := func(name string) bool {
		v, err := strconv.ParseBool(getenv(name))
		if err != nil {
			return true
		}
		return v
	}
	return Flags{
		AbsenceActions:   b("FF_ABSENCE_ACTIONS"),
		AccessActions:    b("FF_ACCESS_ACTIONS"),
		MentorEscalation: b("FF_MENTOR_ESCALATION"),
		FAQFromAnswers:   b("FF_FAQ_FROM_ANSWERS"),
		LLM:              b("FF_LLM"),
		Proactive:        b("FF_PROACTIVE"),
	}
}
