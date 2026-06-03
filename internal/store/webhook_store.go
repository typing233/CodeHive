package store

import (
	"context"
	"database/sql"

	"github.com/codehive/codehive/internal/models"
	"github.com/lib/pq"
)

type WebhookStore struct {
	db *sql.DB
}

func NewWebhookStore(db *sql.DB) *WebhookStore {
	return &WebhookStore{db: db}
}

func (s *WebhookStore) Create(ctx context.Context, wh *models.Webhook) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO webhooks (repo_id, url, secret, events, is_active)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at, updated_at`,
		wh.RepoID, wh.URL, wh.Secret, pq.Array(wh.Events), wh.IsActive,
	).Scan(&wh.ID, &wh.CreatedAt, &wh.UpdatedAt)
}

func (s *WebhookStore) GetByID(ctx context.Context, id int64) (*models.Webhook, error) {
	wh := &models.Webhook{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, repo_id, url, secret, events, is_active, created_at, updated_at
		 FROM webhooks WHERE id = $1`, id,
	).Scan(&wh.ID, &wh.RepoID, &wh.URL, &wh.Secret, pq.Array(&wh.Events), &wh.IsActive, &wh.CreatedAt, &wh.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return wh, nil
}

func (s *WebhookStore) ListByRepo(ctx context.Context, repoID int64) ([]*models.Webhook, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, repo_id, url, secret, events, is_active, created_at, updated_at
		 FROM webhooks WHERE repo_id = $1 ORDER BY created_at DESC`, repoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hooks []*models.Webhook
	for rows.Next() {
		wh := &models.Webhook{}
		if err := rows.Scan(&wh.ID, &wh.RepoID, &wh.URL, &wh.Secret, pq.Array(&wh.Events),
			&wh.IsActive, &wh.CreatedAt, &wh.UpdatedAt); err != nil {
			return nil, err
		}
		hooks = append(hooks, wh)
	}
	return hooks, rows.Err()
}

func (s *WebhookStore) ListByRepoAndEvent(ctx context.Context, repoID int64, event string) ([]*models.Webhook, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, repo_id, url, secret, events, is_active, created_at, updated_at
		 FROM webhooks WHERE repo_id = $1 AND is_active = TRUE AND $2 = ANY(events)`, repoID, event,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hooks []*models.Webhook
	for rows.Next() {
		wh := &models.Webhook{}
		if err := rows.Scan(&wh.ID, &wh.RepoID, &wh.URL, &wh.Secret, pq.Array(&wh.Events),
			&wh.IsActive, &wh.CreatedAt, &wh.UpdatedAt); err != nil {
			return nil, err
		}
		hooks = append(hooks, wh)
	}
	return hooks, rows.Err()
}

func (s *WebhookStore) ListByOwnerAndEvent(ctx context.Context, ownerID int64, event string) ([]*models.Webhook, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.id, w.repo_id, w.url, w.secret, w.events, w.is_active, w.created_at, w.updated_at
		 FROM webhooks w
		 JOIN repositories r ON w.repo_id = r.id
		 WHERE r.owner_id = $1 AND w.is_active = TRUE AND $2 = ANY(w.events)`, ownerID, event,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hooks []*models.Webhook
	for rows.Next() {
		wh := &models.Webhook{}
		if err := rows.Scan(&wh.ID, &wh.RepoID, &wh.URL, &wh.Secret, pq.Array(&wh.Events),
			&wh.IsActive, &wh.CreatedAt, &wh.UpdatedAt); err != nil {
			return nil, err
		}
		hooks = append(hooks, wh)
	}
	return hooks, rows.Err()
}

func (s *WebhookStore) Update(ctx context.Context, wh *models.Webhook) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE webhooks SET url=$1, secret=$2, events=$3, is_active=$4, updated_at=NOW()
		 WHERE id=$5`,
		wh.URL, wh.Secret, pq.Array(wh.Events), wh.IsActive, wh.ID,
	)
	return err
}

func (s *WebhookStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id=$1`, id)
	return err
}

func (s *WebhookStore) RecordDelivery(ctx context.Context, d *models.WebhookDelivery) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO webhook_deliveries (webhook_id, event, payload, response_code, response_body, duration_ms)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, delivered_at`,
		d.WebhookID, d.Event, d.Payload, d.ResponseCode, d.ResponseBody, d.DurationMs,
	).Scan(&d.ID, &d.DeliveredAt)
}

func (s *WebhookStore) ListDeliveries(ctx context.Context, webhookID int64, page, limit int) ([]*models.WebhookDelivery, error) {
	if limit == 0 {
		limit = 20
	}
	if page == 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, webhook_id, event, payload, response_code, response_body, delivered_at, duration_ms
		 FROM webhook_deliveries WHERE webhook_id = $1
		 ORDER BY delivered_at DESC LIMIT $2 OFFSET $3`,
		webhookID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []*models.WebhookDelivery
	for rows.Next() {
		d := &models.WebhookDelivery{}
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.Event, &d.Payload, &d.ResponseCode,
			&d.ResponseBody, &d.DeliveredAt, &d.DurationMs); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}
