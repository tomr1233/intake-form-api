package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tomr1233/intake-form-api/internal/database"
	"github.com/tomr1233/intake-form-api/internal/models"
)

// WebhookRepository persists webhook subscriptions.
type WebhookRepository interface {
	Create(ctx context.Context, w *models.Webhook) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Webhook, error)
	List(ctx context.Context) ([]*models.Webhook, error)
	ListActiveForEvent(ctx context.Context, eventType string) ([]*models.Webhook, error)
	Update(ctx context.Context, id uuid.UUID, upd WebhookUpdate) (*models.Webhook, error)
	RotateSecret(ctx context.Context, id uuid.UUID, secret, secretPrefix string) (*models.Webhook, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// WebhookUpdate carries optional fields for partial updates. nil = leave alone.
type WebhookUpdate struct {
	Name   *string
	URL    *string
	Events []string // nil = unchanged; empty = invalid (handler should reject)
	Active *bool
}

type webhookRepo struct {
	db *database.DB
}

func NewWebhookRepository(db *database.DB) WebhookRepository {
	return &webhookRepo{db: db}
}

func (r *webhookRepo) Create(ctx context.Context, w *models.Webhook) error {
	query := `
		INSERT INTO webhooks (id, name, url, secret, secret_prefix, events, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := r.db.Pool.Exec(ctx, query,
		w.ID, w.Name, w.URL, w.Secret, w.SecretPrefix, w.Events, w.Active, w.CreatedAt, w.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting webhook: %w", err)
	}
	return nil
}

func (r *webhookRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Webhook, error) {
	query := `
		SELECT id, name, url, secret, secret_prefix, events, active, created_at, updated_at
		FROM webhooks WHERE id = $1`
	var w models.Webhook
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&w.ID, &w.Name, &w.URL, &w.Secret, &w.SecretPrefix, &w.Events, &w.Active, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying webhook: %w", err)
	}
	return &w, nil
}

func (r *webhookRepo) List(ctx context.Context) ([]*models.Webhook, error) {
	query := `
		SELECT id, name, url, secret, secret_prefix, events, active, created_at, updated_at
		FROM webhooks ORDER BY created_at DESC`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing webhooks: %w", err)
	}
	defer rows.Close()
	return scanWebhookRows(rows)
}

func (r *webhookRepo) ListActiveForEvent(ctx context.Context, eventType string) ([]*models.Webhook, error) {
	query := `
		SELECT id, name, url, secret, secret_prefix, events, active, created_at, updated_at
		FROM webhooks
		WHERE active = TRUE AND events @> ARRAY[$1]::TEXT[]`
	rows, err := r.db.Pool.Query(ctx, query, eventType)
	if err != nil {
		return nil, fmt.Errorf("listing active webhooks for event: %w", err)
	}
	defer rows.Close()
	return scanWebhookRows(rows)
}

func (r *webhookRepo) Update(ctx context.Context, id uuid.UUID, upd WebhookUpdate) (*models.Webhook, error) {
	// COALESCE-style partial update.
	query := `
		UPDATE webhooks
		SET name = COALESCE($2, name),
		    url = COALESCE($3, url),
		    events = COALESCE($4, events),
		    active = COALESCE($5, active),
		    updated_at = $6
		WHERE id = $1
		RETURNING id, name, url, secret, secret_prefix, events, active, created_at, updated_at`

	var eventsArg interface{}
	if upd.Events != nil {
		eventsArg = upd.Events
	}

	var w models.Webhook
	err := r.db.Pool.QueryRow(ctx, query, id, upd.Name, upd.URL, eventsArg, upd.Active, time.Now()).Scan(
		&w.ID, &w.Name, &w.URL, &w.Secret, &w.SecretPrefix, &w.Events, &w.Active, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("updating webhook: %w", err)
	}
	return &w, nil
}

func (r *webhookRepo) RotateSecret(ctx context.Context, id uuid.UUID, secret, secretPrefix string) (*models.Webhook, error) {
	query := `
		UPDATE webhooks
		SET secret = $2, secret_prefix = $3, updated_at = $4
		WHERE id = $1
		RETURNING id, name, url, secret, secret_prefix, events, active, created_at, updated_at`
	var w models.Webhook
	err := r.db.Pool.QueryRow(ctx, query, id, secret, secretPrefix, time.Now()).Scan(
		&w.ID, &w.Name, &w.URL, &w.Secret, &w.SecretPrefix, &w.Events, &w.Active, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("rotating webhook secret: %w", err)
	}
	return &w, nil
}

func (r *webhookRepo) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.Pool.Exec(ctx, `DELETE FROM webhooks WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting webhook: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanWebhookRows(rows pgx.Rows) ([]*models.Webhook, error) {
	var out []*models.Webhook
	for rows.Next() {
		var w models.Webhook
		if err := rows.Scan(
			&w.ID, &w.Name, &w.URL, &w.Secret, &w.SecretPrefix, &w.Events, &w.Active, &w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning webhook row: %w", err)
		}
		out = append(out, &w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating webhook rows: %w", err)
	}
	return out, nil
}
