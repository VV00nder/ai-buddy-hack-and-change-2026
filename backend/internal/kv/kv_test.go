package kv

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"aibuddy/internal/domain"
)

func contract(t *testing.T, k KV, advance func(time.Duration)) {
	ctx := context.Background()
	if err := k.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	s := Session{User: domain.User{ID: "u1", Name: "Анна"}, Token: "tok"}
	if err := k.SetSession(ctx, "sid", s, time.Hour); err != nil {
		t.Fatal(err)
	}
	got, err := k.GetSession(ctx, "sid")
	if err != nil || got.User.Name != "Анна" || got.Token != "tok" {
		t.Fatalf("get: %+v %v", got, err)
	}
	_ = k.DeleteSession(ctx, "sid")
	if _, err := k.GetSession(ctx, "sid"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("deleted: %v", err)
	}
	key := "u1:" + time.Now().Format(time.RFC3339Nano)
	for i := 0; i < 3; i++ {
		if ok, _ := k.Allow(ctx, key, 3, time.Minute); !ok {
			t.Fatal("within limit")
		}
	}
	if ok, _ := k.Allow(ctx, key, 3, time.Minute); ok {
		t.Fatal("over limit")
	}
	if advance != nil {
		_ = k.SetSession(ctx, "short", s, time.Minute)
		advance(2 * time.Minute)
		if _, err := k.GetSession(ctx, "short"); !errors.Is(err, ErrNoSession) {
			t.Fatal("expired")
		}
		if ok, _ := k.Allow(ctx, key, 3, time.Minute); !ok {
			t.Fatal("window reset")
		}
	}
}

func TestMemory(t *testing.T) {
	now := time.Now()
	m := NewMemory(func() time.Time { return now })
	contract(t, m, func(d time.Duration) { now = now.Add(d) })
	if NewMemory(nil).now == nil {
		t.Fatal("default clock")
	}
}

func TestRedis(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL не задан — тест Redis пропущен")
	}
	r, err := OpenRedis(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	contract(t, r, nil)
	// повреждённые данные сессии
	_ = r.c.Set(context.Background(), "buddy:session:bad", "{", time.Minute).Err()
	if _, err := r.GetSession(context.Background(), "bad"); err == nil {
		t.Fatal("bad json")
	}
	_ = r.Close()
	if _, err := r.GetSession(context.Background(), "x"); err == nil || errors.Is(err, ErrNoSession) {
		t.Fatal("closed")
	}
	if _, err := r.Allow(context.Background(), "x", 1, time.Second); err == nil {
		t.Fatal("closed allow")
	}
}

func TestRedisOpenErrors(t *testing.T) {
	if _, err := OpenRedis(context.Background(), "bad://"); err == nil {
		t.Fatal("bad url")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := OpenRedis(ctx, "redis://127.0.0.1:1/0"); err == nil {
		t.Fatal("unreachable")
	}
}
