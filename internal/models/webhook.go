package models

import (
	"encoding/json"
	"time"
)

type Webhook struct {
	ID        int64     `json:"id"`
	RepoID    int64     `json:"repo_id"`
	URL       string    `json:"url"`
	Secret    string    `json:"-"`
	Events    []string  `json:"events"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type WebhookDelivery struct {
	ID           int64           `json:"id"`
	WebhookID    int64           `json:"webhook_id"`
	Event        string          `json:"event"`
	Payload      json.RawMessage `json:"payload"`
	ResponseCode *int            `json:"response_code"`
	ResponseBody *string         `json:"response_body"`
	DeliveredAt  time.Time       `json:"delivered_at"`
	DurationMs   int             `json:"duration_ms"`
}
