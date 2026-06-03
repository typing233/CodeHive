package models

import (
	"encoding/json"
	"time"
)

type AuditEntry struct {
	ID         int64           `json:"id"`
	ActorID    *int64          `json:"actor_id"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type"`
	TargetID   *int64          `json:"target_id"`
	Metadata   json.RawMessage `json:"metadata"`
	IPAddress  string          `json:"ip_address"`
	CreatedAt  time.Time       `json:"created_at"`

	Actor *User `json:"actor,omitempty"`
}

type AuditFilter struct {
	ActorID    *int64
	Action     string
	TargetType string
	TargetID   *int64
	Since      *time.Time
	Until      *time.Time
	Page       int
	Limit      int
}
