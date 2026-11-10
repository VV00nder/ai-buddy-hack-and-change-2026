// Package config — настройки из переменных окружения.
package config

import (
	"os"
	"strconv"
	"time"
	_ "time/tzdata" // часовой пояс внутри минимального контейнера
)

// Buddy — настройки сервиса AI-buddy.
type Buddy struct {
	Addr         string
	CorpURL      string
	DatabaseURL  string
	RedisURL     string
	WebDir       string
	CookieSecure bool
	Location     *time.Location

	LLMBaseURL     string
	LLMAPIKey      string
	LLMModel       string
	LLMTemperature float64
	LLMTimeout     time.Duration
	AgentTimeout   time.Duration
	MsgRateLimit   int
}

// Corp — настройки имитации корпоративных сервисов.
type Corp struct {
	Addr           string
	IdentitySecret string
	DemoPassword   string
	AdminToken     string
	Location       *time.Location
}

// Getenv — источник переменных (подменяется в тестах).
type Getenv func(string) string

func str(g Getenv, k, def string) string {
	if v := g(k); v != "" {
		return v
	}
	return def
}

func num(g Getenv, k string, def float64) float64 {
	if v, err := strconv.ParseFloat(g(k), 64); err == nil {
		return v
	}
	return def
}

func loc(g Getenv) *time.Location {
	l, err := time.LoadLocation(str(g, "APP_TZ", "Europe/Moscow"))
	if err != nil {
		return time.UTC
	}
	return l
}

// LoadBuddy читает настройки AI-buddy.
func LoadBuddy(g Getenv) Buddy {
	if g == nil {
		g = os.Getenv
	}
	return Buddy{
		Addr:           str(g, "BUDDY_ADDR", ":8080"),
		CorpURL:        str(g, "CORP_URL", "http://localhost:8090"),
		DatabaseURL:    g("DATABASE_URL"),
		RedisURL:       g("REDIS_URL"),
		WebDir:         str(g, "WEB_DIR", "../web/dist"),
		CookieSecure:   g("COOKIE_SECURE") == "true",
		Location:       loc(g),
		LLMBaseURL:     str(g, "LLM_BASE_URL", "https://api.openai.com/v1"),
		LLMAPIKey:      g("LLM_API_KEY"),
		LLMModel:       str(g, "LLM_MODEL", "gpt-4o-mini"),
		LLMTemperature: num(g, "LLM_TEMPERATURE", 0.2),
		LLMTimeout:     time.Duration(num(g, "LLM_TIMEOUT_SECONDS", 40)) * time.Second,
		AgentTimeout:   time.Duration(num(g, "AGENT_TIMEOUT_SECONDS", 60)) * time.Second,
		MsgRateLimit:   int(num(g, "MSG_RATE_LIMIT_PER_MIN", 20)),
	}
}

// LoadCorp читает настройки имитации.
func LoadCorp(g Getenv) Corp {
	if g == nil {
		g = os.Getenv
	}
	return Corp{
		Addr:           str(g, "CORP_ADDR", ":8090"),
		IdentitySecret: str(g, "IDENTITY_SECRET", "dev-only-secret-change-me"),
		DemoPassword:   str(g, "DEMO_PASSWORD", "buddy2026"),
		AdminToken:     str(g, "CORP_ADMIN_TOKEN", "demo-admin"),
		Location:       loc(g),
	}
}
