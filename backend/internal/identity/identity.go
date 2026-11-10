// Package identity — подписанный токен личности сотрудника.
//
// В Авито авторизация и ролевая модель уже существуют; прототип имитирует их
// компактным HMAC-токеном. Корпоративные сервисы проверяют подпись и роль сами,
// поэтому права соблюдаются даже если агент (или LLM) попросит лишнее.
package identity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"aibuddy/internal/domain"
)

// Claims — содержимое токена.
type Claims struct {
	Sub      string      `json:"sub"`
	Login    string      `json:"login"`
	Name     string      `json:"name"`
	Role     domain.Role `json:"role"`
	Team     string      `json:"team"`
	MentorID string      `json:"mentor,omitempty"`
	LeadID   string      `json:"lead,omitempty"`
	Exp      int64       `json:"exp"`
}

// Errors.
var (
	ErrMalformed = errors.New("identity: malformed token")
	ErrSignature = errors.New("identity: bad signature")
	ErrExpired   = errors.New("identity: token expired")
)

// Signer подписывает и проверяет токены.
type Signer struct {
	key []byte
	now func() time.Time
}

// NewSigner создаёт подписчика с общим секретом.
func NewSigner(secret string, now func() time.Time) *Signer {
	if now == nil {
		now = time.Now
	}
	return &Signer{key: []byte(secret), now: now}
}

var enc = base64.RawURLEncoding

// Issue выпускает токен для пользователя.
func (s *Signer) Issue(u domain.User, ttl time.Duration) string {
	c := Claims{Sub: u.ID, Login: u.Login, Name: u.Name, Role: u.Role, Team: u.Team,
		MentorID: u.MentorID, LeadID: u.LeadID, Exp: s.now().Add(ttl).Unix()}
	body, _ := json.Marshal(c)
	payload := enc.EncodeToString(body)
	return payload + "." + enc.EncodeToString(s.mac(payload))
}

// Verify проверяет подпись и срок действия.
func (s *Signer) Verify(token string) (Claims, error) {
	var c Claims
	payload, sig, ok := strings.Cut(token, ".")
	if !ok || payload == "" || sig == "" {
		return c, ErrMalformed
	}
	got, err := enc.DecodeString(sig)
	if err != nil {
		return c, ErrMalformed
	}
	if !hmac.Equal(got, s.mac(payload)) {
		return c, ErrSignature
	}
	body, err := enc.DecodeString(payload)
	if err != nil {
		return c, ErrMalformed
	}
	if err := json.Unmarshal(body, &c); err != nil {
		return c, ErrMalformed
	}
	if s.now().Unix() > c.Exp {
		return c, ErrExpired
	}
	return c, nil
}

func (s *Signer) mac(payload string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(payload))
	return m.Sum(nil)
}
