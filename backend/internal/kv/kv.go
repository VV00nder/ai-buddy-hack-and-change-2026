// Package kv — сессии и ограничение частоты запросов: Redis в рабочем
// режиме, память — для локального запуска и тестов.
package kv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"aibuddy/internal/domain"
)

// ErrNoSession — сессия не найдена или истекла.
var ErrNoSession = errors.New("session not found")

// Session — сессия пользователя AI-buddy.
type Session struct {
	User  domain.User `json:"user"`
	Token string      `json:"token"` // токен личности от SSO, передаётся корпоративным сервисам
}

// KV — хранилище сессий и лимитов.
type KV interface {
	Ping(ctx context.Context) error
	SetSession(ctx context.Context, id string, s Session, ttl time.Duration) error
	GetSession(ctx context.Context, id string) (Session, error)
	DeleteSession(ctx context.Context, id string) error
	// Allow — фиксированное окно: не больше limit событий за window.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// Memory — реализация в памяти.
type Memory struct {
	mu       sync.Mutex
	now      func() time.Time
	sessions map[string]memSession
	counters map[string]memCounter
}

type memSession struct {
	s   Session
	exp time.Time
}

type memCounter struct {
	n   int
	exp time.Time
}

// NewMemory создаёт хранилище в памяти.
func NewMemory(now func() time.Time) *Memory {
	if now == nil {
		now = time.Now
	}
	return &Memory{now: now, sessions: map[string]memSession{}, counters: map[string]memCounter{}}
}

func (m *Memory) Ping(context.Context) error { return nil }

func (m *Memory) SetSession(_ context.Context, id string, s Session, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[id] = memSession{s: s, exp: m.now().Add(ttl)}
	return nil
}

func (m *Memory) GetSession(_ context.Context, id string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ms, ok := m.sessions[id]
	if !ok || m.now().After(ms.exp) {
		delete(m.sessions, id)
		return Session{}, ErrNoSession
	}
	return ms.s, nil
}

func (m *Memory) DeleteSession(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	return nil
}

func (m *Memory) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.counters[key]
	if m.now().After(c.exp) {
		c = memCounter{exp: m.now().Add(window)}
	}
	c.n++
	m.counters[key] = c
	return c.n <= limit, nil
}

// Redis — реализация на Redis.
type Redis struct{ c *redis.Client }

// OpenRedis подключается к Redis по URL (redis://host:6379/0).
func OpenRedis(ctx context.Context, url string) (*Redis, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	c := redis.NewClient(opt)
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("redis: %w", err)
	}
	return &Redis{c: c}, nil
}

// Close закрывает клиента.
func (r *Redis) Close() error { return r.c.Close() }

func (r *Redis) Ping(ctx context.Context) error { return r.c.Ping(ctx).Err() }

func (r *Redis) SetSession(ctx context.Context, id string, s Session, ttl time.Duration) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return r.c.Set(ctx, "buddy:session:"+id, b, ttl).Err()
}

func (r *Redis) GetSession(ctx context.Context, id string) (Session, error) {
	var s Session
	b, err := r.c.Get(ctx, "buddy:session:"+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return s, ErrNoSession
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(b, &s)
	return s, err
}

func (r *Redis) DeleteSession(ctx context.Context, id string) error {
	return r.c.Del(ctx, "buddy:session:"+id).Err()
}

func (r *Redis) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	k := "buddy:rl:" + key
	pipe := r.c.TxPipeline()
	incr := pipe.Incr(ctx, k)
	pipe.ExpireNX(ctx, k, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return incr.Val() <= int64(limit), nil
}
