package models

import "time"

type Repository struct {
	ID            int64     `json:"id"`
	OwnerID       int64     `json:"owner_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	IsPrivate     bool      `json:"is_private"`
	DefaultBranch string    `json:"default_branch"`
	DiskPath      string    `json:"-"`
	SizeBytes     int64     `json:"size_bytes"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	Owner *User `json:"owner,omitempty"`
}

type RepoCollaborator struct {
	ID        int64     `json:"id"`
	RepoID    int64     `json:"repo_id"`
	UserID    int64     `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`

	User *User `json:"user,omitempty"`
}
