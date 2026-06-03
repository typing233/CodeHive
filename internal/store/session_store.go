package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"

	"github.com/codehive/codehive/internal/models"
)

type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

func (s *SessionStore) Create(ctx context.Context, session *models.Session) error {
	id, err := generateSessionID()
	if err != nil {
		return err
	}
	session.ID = id
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, ip_address, user_agent, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		session.ID, session.UserID, session.IPAddress, session.UserAgent, session.ExpiresAt,
	)
	return err
}

func (s *SessionStore) Get(ctx context.Context, id string) (*models.Session, error) {
	sess := &models.Session{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, ip_address, user_agent, expires_at, created_at
		 FROM sessions WHERE id = $1 AND expires_at > NOW()`, id,
	).Scan(&sess.ID, &sess.UserID, &sess.IPAddress, &sess.UserAgent, &sess.ExpiresAt, &sess.CreatedAt)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *SessionStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = $1`, id)
	return err
}

func (s *SessionStore) DeleteExpired(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < NOW()`)
	return err
}

func (s *SessionStore) DeleteByUser(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}

func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func NewSessionExpiry(maxAge int) time.Time {
	return time.Now().Add(time.Duration(maxAge) * time.Second)
}
