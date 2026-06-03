package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/codehive/codehive/internal/models"
	"golang.org/x/crypto/ssh"
)

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Create(ctx context.Context, user *models.User) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO users (username, email, password_hash, full_name, is_admin)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at, updated_at`,
		user.Username, user.Email, user.PasswordHash, user.FullName, user.IsAdmin,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
}

func (s *UserStore) GetByID(ctx context.Context, id int64) (*models.User, error) {
	u := &models.User{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, email, password_hash, full_name, is_admin, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.FullName, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *UserStore) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	u := &models.User{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, email, password_hash, full_name, is_admin, created_at, updated_at
		 FROM users WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.FullName, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *UserStore) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	u := &models.User{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, email, password_hash, full_name, is_admin, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.FullName, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *UserStore) Update(ctx context.Context, user *models.User) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET username=$1, email=$2, full_name=$3, updated_at=NOW() WHERE id=$4`,
		user.Username, user.Email, user.FullName, user.ID,
	)
	return err
}

func (s *UserStore) UpdatePassword(ctx context.Context, userID int64, hash string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2`, hash, userID,
	)
	return err
}

func (s *UserStore) AddSSHKey(ctx context.Context, key *models.SSHKey) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO ssh_keys (user_id, name, fingerprint, public_key)
		 VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		key.UserID, key.Name, key.Fingerprint, key.PublicKey,
	).Scan(&key.ID, &key.CreatedAt)
}

func (s *UserStore) ListSSHKeys(ctx context.Context, userID int64) ([]*models.SSHKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, fingerprint, public_key, created_at
		 FROM ssh_keys WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*models.SSHKey
	for rows.Next() {
		k := &models.SSHKey{}
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.Fingerprint, &k.PublicKey, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *UserStore) DeleteSSHKey(ctx context.Context, id, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM ssh_keys WHERE id = $1 AND user_id = $2`, id, userID,
	)
	return err
}

func (s *UserStore) GetUserByFingerprint(ctx context.Context, fp string) (*models.User, error) {
	u := &models.User{}
	err := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.username, u.email, u.password_hash, u.full_name, u.is_admin, u.created_at, u.updated_at
		 FROM users u JOIN ssh_keys k ON u.id = k.user_id WHERE k.fingerprint = $1`, fp,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.FullName, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *UserStore) ListAll(ctx context.Context) ([]*models.User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, username, email, full_name, is_admin, created_at FROM users ORDER BY username`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		u := &models.User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.FullName, &u.IsAdmin, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func ComputeSSHFingerprint(pubKey ssh.PublicKey) string {
	h := sha256.Sum256(pubKey.Marshal())
	return fmt.Sprintf("SHA256:%s", hex.EncodeToString(h[:]))
}
