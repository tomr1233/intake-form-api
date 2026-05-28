package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tomr1233/intake-form-api/internal/database"
	"github.com/tomr1233/intake-form-api/internal/models"
)

// WebhookDeliveryRepository persists per-attempt delivery records.
type WebhookDeliveryRepository interface {
	Create(ctx context.Context, d *models.WebhookDelivery) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.WebhookDelivery, error)
	List(ctx context.Context, webhookID uuid.UUID, before *uuid.UUID, limit int) ([]*models.WebhookDelivery, error)
	MarkSuccess(ctx context.Context, id uuid.UUID, attempts int, statusCode int, respBody string) error
	MarkPendingRetry(ctx context.Context, id uuid.UUID, attempts int, statusCode int, errMsg, respBody string, nextAt time.Time) error
	MarkFailed(ctx context.Context, id uuid.UUID, attempts int, statusCode int, errMsg, respBody string) error
}

type webhookDeliveryRepo struct {
	db *database.DB
}

func NewWebhookDeliveryRepository(db *database.DB) WebhookDeliveryRepository {
	return &webhookDeliveryRepo{db: db}
}

func (r *webhookDeliveryRepo) Create(ctx context.Context, d *models.WebhookDelivery) error {
	query := `
		INSERT INTO webhook_deliveries (
			id, webhook_id, event_type, event_id, payload, status, attempts,
			last_status_code, last_error, last_response_body, next_attempt_at,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`
	_, err := r.db.Pool.Exec(ctx, query,
		d.ID, d.WebhookID, d.EventType, d.EventID, []byte(d.Payload), d.Status, d.Attempts,
		d.LastStatusCode, d.LastError, d.LastResponseBody, d.NextAttemptAt,
		d.CreatedAt, d.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting webhook delivery: %w", err)
	}
	return nil
}

func (r *webhookDeliveryRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.WebhookDelivery, error) {
	query := `
		SELECT id, webhook_id, event_type, event_id, payload, status, attempts,
		       last_status_code, last_error, last_response_body, next_attempt_at,
		       created_at, updated_at
		FROM webhook_deliveries WHERE id = $1`
	var d models.WebhookDelivery
	var payload []byte
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&d.ID, &d.WebhookID, &d.EventType, &d.EventID, &payload, &d.Status, &d.Attempts,
		&d.LastStatusCode, &d.LastError, &d.LastResponseBody, &d.NextAttemptAt,
		&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying webhook delivery: %w", err)
	}
	d.Payload = json.RawMessage(payload)
	return &d, nil
}

func (r *webhookDeliveryRepo) List(ctx context.Context, webhookID uuid.UUID, before *uuid.UUID, limit int) ([]*models.WebhookDelivery, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	var rows pgx.Rows
	var err error

	if before == nil {
		rows, err = r.db.Pool.Query(ctx, `
			SELECT id, webhook_id, event_type, event_id, payload, status, attempts,
			       last_status_code, last_error, last_response_body, next_attempt_at,
			       created_at, updated_at
			FROM webhook_deliveries
			WHERE webhook_id = $1
			ORDER BY created_at DESC
			LIMIT $2`, webhookID, limit)
	} else {
		rows, err = r.db.Pool.Query(ctx, `
			SELECT id, webhook_id, event_type, event_id, payload, status, attempts,
			       last_status_code, last_error, last_response_body, next_attempt_at,
			       created_at, updated_at
			FROM webhook_deliveries
			WHERE webhook_id = $1
			  AND created_at < (SELECT created_at FROM webhook_deliveries WHERE id = $2 AND webhook_id = $1)
			ORDER BY created_at DESC
			LIMIT $3`, webhookID, *before, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("listing webhook deliveries: %w", err)
	}
	defer rows.Close()

	var out []*models.WebhookDelivery
	for rows.Next() {
		var d models.WebhookDelivery
		var payload []byte
		if err := rows.Scan(
			&d.ID, &d.WebhookID, &d.EventType, &d.EventID, &payload, &d.Status, &d.Attempts,
			&d.LastStatusCode, &d.LastError, &d.LastResponseBody, &d.NextAttemptAt,
			&d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning delivery row: %w", err)
		}
		d.Payload = json.RawMessage(payload)
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating delivery rows: %w", err)
	}
	return out, nil
}

func (r *webhookDeliveryRepo) MarkSuccess(ctx context.Context, id uuid.UUID, attempts int, statusCode int, respBody string) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE webhook_deliveries
		SET status = 'success',
		    attempts = $2,
		    last_status_code = $3,
		    last_error = NULL,
		    last_response_body = $4,
		    next_attempt_at = NULL,
		    updated_at = $5
		WHERE id = $1`, id, attempts, statusCode, respBody, time.Now())
	if err != nil {
		return fmt.Errorf("marking delivery success: %w", err)
	}
	return nil
}

func (r *webhookDeliveryRepo) MarkPendingRetry(ctx context.Context, id uuid.UUID, attempts int, statusCode int, errMsg, respBody string, nextAt time.Time) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE webhook_deliveries
		SET status = 'pending',
		    attempts = $2,
		    last_status_code = NULLIF($3, 0),
		    last_error = $4,
		    last_response_body = $5,
		    next_attempt_at = $6,
		    updated_at = $7
		WHERE id = $1`, id, attempts, statusCode, errMsg, respBody, nextAt, time.Now())
	if err != nil {
		return fmt.Errorf("marking delivery pending retry: %w", err)
	}
	return nil
}

func (r *webhookDeliveryRepo) MarkFailed(ctx context.Context, id uuid.UUID, attempts int, statusCode int, errMsg, respBody string) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE webhook_deliveries
		SET status = 'failed',
		    attempts = $2,
		    last_status_code = NULLIF($3, 0),
		    last_error = $4,
		    last_response_body = $5,
		    next_attempt_at = NULL,
		    updated_at = $6
		WHERE id = $1`, id, attempts, statusCode, errMsg, respBody, time.Now())
	if err != nil {
		return fmt.Errorf("marking delivery failed: %w", err)
	}
	return nil
}
