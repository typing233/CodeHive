package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/codehive/codehive/internal/models"
)

type AuditStore struct {
	db *sql.DB
}

func NewAuditStore(db *sql.DB) *AuditStore {
	return &AuditStore{db: db}
}

func (s *AuditStore) Log(ctx context.Context, actorID *int64, action, targetType string, targetID *int64, metadata map[string]interface{}, ip string) error {
	meta, _ := json.Marshal(metadata)
	if metadata == nil {
		meta = []byte("{}")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log (actor_id, action, target_type, target_id, metadata, ip_address)
		 VALUES ($1, $2, $3, $4, $5, $6::inet)`,
		actorID, action, targetType, targetID, meta, nullableString(ip),
	)
	return err
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func (s *AuditStore) List(ctx context.Context, filter models.AuditFilter) ([]*models.AuditEntry, int, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	argN := 1

	if filter.ActorID != nil {
		where = append(where, fmt.Sprintf("a.actor_id = $%d", argN))
		args = append(args, *filter.ActorID)
		argN++
	}
	if filter.Action != "" {
		where = append(where, fmt.Sprintf("a.action = $%d", argN))
		args = append(args, filter.Action)
		argN++
	}
	if filter.TargetType != "" {
		where = append(where, fmt.Sprintf("a.target_type = $%d", argN))
		args = append(args, filter.TargetType)
		argN++
	}
	if filter.TargetID != nil {
		where = append(where, fmt.Sprintf("a.target_id = $%d", argN))
		args = append(args, *filter.TargetID)
		argN++
	}
	if filter.Since != nil {
		where = append(where, fmt.Sprintf("a.created_at >= $%d", argN))
		args = append(args, *filter.Since)
		argN++
	}
	if filter.Until != nil {
		where = append(where, fmt.Sprintf("a.created_at <= $%d", argN))
		args = append(args, *filter.Until)
		argN++
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	countQ := fmt.Sprintf("SELECT COUNT(*) FROM audit_log a WHERE %s", whereClause)
	s.db.QueryRowContext(ctx, countQ, args...).Scan(&total)

	if filter.Limit == 0 {
		filter.Limit = 50
	}
	if filter.Page == 0 {
		filter.Page = 1
	}
	offset := (filter.Page - 1) * filter.Limit

	query := fmt.Sprintf(
		`SELECT a.id, a.actor_id, a.action, a.target_type, a.target_id, a.metadata, a.ip_address, a.created_at,
		        COALESCE(u.username, '')
		 FROM audit_log a LEFT JOIN users u ON a.actor_id = u.id
		 WHERE %s ORDER BY a.created_at DESC LIMIT $%d OFFSET $%d`,
		whereClause, argN, argN+1,
	)
	args = append(args, filter.Limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []*models.AuditEntry
	for rows.Next() {
		e := &models.AuditEntry{}
		var actorID sql.NullInt64
		var targetID sql.NullInt64
		var ip sql.NullString
		var username string
		if err := rows.Scan(&e.ID, &actorID, &e.Action, &e.TargetType, &targetID,
			&e.Metadata, &ip, &e.CreatedAt, &username); err != nil {
			return nil, 0, err
		}
		if actorID.Valid {
			e.ActorID = &actorID.Int64
			e.Actor = &models.User{ID: actorID.Int64, Username: username}
		}
		if targetID.Valid {
			e.TargetID = &targetID.Int64
		}
		if ip.Valid {
			e.IPAddress = ip.String
		}
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}
