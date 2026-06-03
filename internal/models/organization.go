package models

import "time"

type Organization struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Description string    `json:"description"`
	IsPublic    bool      `json:"is_public"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type OrgMember struct {
	ID        int64     `json:"id"`
	OrgID     int64     `json:"org_id"`
	UserID    int64     `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`

	User *User `json:"user,omitempty"`
}

type Team struct {
	ID          int64     `json:"id"`
	OrgID       int64     `json:"org_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Permission  string    `json:"permission"`
	CreatedAt   time.Time `json:"created_at"`

	Members []*User `json:"members,omitempty"`
}
