package identity

import (
	"strings"
	"testing"
	"time"

	"aibuddy/internal/domain"
)

func TestIssueVerify(t *testing.T) {
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	s := NewSigner("secret", func() time.Time { return now })
	tok := s.Issue(domain.User{ID: "u1", Login: "anna", Role: domain.RoleIntern, Team: "search"}, time.Hour)
	c, err := s.Verify(tok)
	if err != nil || c.Sub != "u1" || c.Role != domain.RoleIntern || c.Team != "search" {
		t.Fatalf("verify: %+v %v", c, err)
	}
	other := NewSigner("other", func() time.Time { return now })
	if _, err := other.Verify(tok); err != ErrSignature {
		t.Fatalf("want signature error, got %v", err)
	}
	later := NewSigner("secret", func() time.Time { return now.Add(2 * time.Hour) })
	if _, err := later.Verify(tok); err != ErrExpired {
		t.Fatalf("want expired, got %v", err)
	}
	payload, sig, _ := strings.Cut(tok, ".")
	for _, bad := range []string{"", "abc", "abc.", ".abc", payload + ".!!!", "!!!." + sig} {
		if _, err := s.Verify(bad); err == nil {
			t.Fatalf("token %q must fail", bad)
		}
	}
	// подделка роли: payload изменён, подпись старая
	forged := enc.EncodeToString([]byte(`{"sub":"u1","role":"lead","exp":9999999999}`)) + "." + sig
	if _, err := s.Verify(forged); err != ErrSignature {
		t.Fatalf("forged token must fail, got %v", err)
	}
	// валидная подпись, но битый JSON
	p := enc.EncodeToString([]byte("not json"))
	if _, err := s.Verify(p + "." + enc.EncodeToString(s.mac(p))); err != ErrMalformed {
		t.Fatalf("want malformed, got %v", err)
	}
	if NewSigner("x", nil).now == nil {
		t.Fatal("default clock")
	}
}
