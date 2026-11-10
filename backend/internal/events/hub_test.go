package events

import "testing"

func TestHub(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe("u")
	h.Publish("u", Update{Type: "message"})
	h.Publish("other", Update{Type: "x"})
	if u := <-ch; u.Type != "message" {
		t.Fatal(u)
	}
	for i := 0; i < 40; i++ { // переполнение буфера не блокирует
		h.Publish("u", Update{Type: "spam"})
	}
	if h.Subscribers("u") != 1 {
		t.Fatal("subs")
	}
	cancel()
	cancel()
	if h.Subscribers("u") != 0 {
		t.Fatal("cancel")
	}
}
