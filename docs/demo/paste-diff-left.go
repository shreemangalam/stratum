package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

// Session holds an authenticated user session.
type Session struct {
	ID        string
	UserID    string
	Token     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// NewSession creates a session that expires after the given duration.
func NewSession(userID string, ttl time.Duration) (*Session, error) {
	token, err := generateToken(32)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	return &Session{
		UserID:    userID,
		Token:     token,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}, nil
}

// IsExpired reports whether the session has passed its expiry time.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// Validate checks the session token and expiry.
func (s *Session) Validate(token string) error {
	if s.Token != token {
		return errors.New("invalid token")
	}
	if s.IsExpired() {
		return errors.New("session expired")
	}
	return nil
}

func generateToken(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
