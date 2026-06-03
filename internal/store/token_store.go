package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"

	"github.com/codehive/codehive/internal/models"
)

type TokenStore struct {
	db *sql.DB
}

func NewTokenStore(db *sql.DB) *TokenStore {
	return &TokenStore{db: db}
}

func GenerateToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = "ch_" + hex.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	return raw, hash, nil
}

func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func (s *TokenStore) Create(ctx context.Context, token *models.AccessToken) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO access_tokens (user_id, name, token_hash, scopes, expires_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`,
		token.UserID, token.Name, token.TokenHash, token.Scopes, token.ExpiresAt,
	).Scan(&token.ID, &token.CreatedAt)
}

func (s *TokenStore) GetByHash(ctx context.Context, hash string) (*models.AccessToken, error) {
	t := &models.AccessToken{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, token_hash, scopes, last_used, expires_at, created_at
		 FROM access_tokens WHERE token_hash = $1
		 AND (expires_at IS NULL OR expires_at > NOW())`, hash,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.Scopes, &t.LastUsed, &t.ExpiresAt, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	s.db.ExecContext(ctx, `UPDATE access_tokens SET last_used=NOW() WHERE id=$1`, t.ID)
	return t, nil
}

func (s *TokenStore) List(ctx context.Context, userID int64) ([]*models.AccessToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, scopes, last_used, expires_at, created_at
		 FROM access_tokens WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*models.AccessToken
	for rows.Next() {
		t := &models.AccessToken{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Scopes, &t.LastUsed, &t.ExpiresAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (s *TokenStore) Delete(ctx context.Context, id, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM access_tokens WHERE id=$1 AND user_id=$2`, id, userID,
	)
	return err
}
