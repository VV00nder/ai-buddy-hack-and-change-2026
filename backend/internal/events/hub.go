// Package events — рассылка обновлений в интерфейс (SSE) и продуктовая
// аналитика (события в формате, совместимом с clickstream).
package events

import (
	"sync"
)

// Update — обновление для интерфейса.
type Update struct {
	Type string `json:"type"` // message | action | notification | plan | question | approval
	Data any    `json:"data"`
}

// Hub — подписки пользователей на обновления.
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan Update]struct{}
}

// NewHub создаёт хаб.
func NewHub() *Hub { return &Hub{subs: map[string]map[chan Update]struct{}{}} }

// Subscribe подписывает пользователя; вызовите cancel при отключении.
func (h *Hub) Subscribe(userID string) (<-chan Update, func()) {
	ch := make(chan Update, 32)
	h.mu.Lock()
	if h.subs[userID] == nil {
		h.subs[userID] = map[chan Update]struct{}{}
	}
	h.subs[userID][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if _, ok := h.subs[userID][ch]; ok {
			delete(h.subs[userID], ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

// Publish отправляет обновление всем вкладкам пользователя; медленные
// подписчики не блокируют отправителя.
func (h *Hub) Publish(userID string, u Update) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[userID] {
		select {
		case ch <- u:
		default:
		}
	}
}

// Subscribers — число подписок пользователя.
func (h *Hub) Subscribers(userID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[userID])
}
