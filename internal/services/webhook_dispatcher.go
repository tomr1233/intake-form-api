package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tomr1233/intake-form-api/internal/models"
)

const (
	maxAttempts           = 4
	defaultRequestTimeout = 10 * time.Second
	maxResponseBodyBytes  = 4 * 1024
	userAgent             = "intake-form-api-webhook/1.0"
)

// WebhookLister is the subset of the webhook repository the dispatcher needs.
// Defined here so tests can satisfy it with a tiny fake.
type WebhookLister interface {
	ListActiveForEvent(ctx context.Context, eventType string) ([]*models.Webhook, error)
}

// DeliveryWriter is the subset of the delivery repository the dispatcher needs.
type DeliveryWriter interface {
	Create(ctx context.Context, d *models.WebhookDelivery) error
	MarkSuccess(ctx context.Context, id uuid.UUID, attempts int, statusCode int, respBody string) error
	MarkPendingRetry(ctx context.Context, id uuid.UUID, attempts int, statusCode int, errMsg, respBody string, nextAt time.Time) error
	MarkFailed(ctx context.Context, id uuid.UUID, attempts int, statusCode int, errMsg, respBody string) error
}

// DispatcherOptions configures a Dispatcher.
type DispatcherOptions struct {
	AllowPrivateIPs bool
	BackoffSchedule []time.Duration // exactly 3 entries; defaults to {5s, 30s, 2min}
	RequestTimeout  time.Duration   // defaults to 10s
}

// Dispatcher fans out events to active webhooks.
type Dispatcher struct {
	webhooks   WebhookLister
	deliveries DeliveryWriter
	httpClient *http.Client
	opts       DispatcherOptions
}

// NewDispatcher builds a Dispatcher with an SSRF-aware HTTP client.
func NewDispatcher(
	webhooks WebhookLister,
	deliveries DeliveryWriter,
	opts DispatcherOptions,
) *Dispatcher {
	if opts.RequestTimeout == 0 {
		opts.RequestTimeout = defaultRequestTimeout
	}
	if len(opts.BackoffSchedule) == 0 {
		opts.BackoffSchedule = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute}
	}

	dialer := &net.Dialer{Timeout: opts.RequestTimeout}
	allowPrivate := opts.AllowPrivateIPs

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("resolving %s: %w", host, err)
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("no addresses for %s", host)
			}
			if !allowPrivate {
				for _, ip := range ips {
					if isPrivateAddr(ip) {
						return nil, fmt.Errorf("refusing to dial private/loopback address %s for host %s", ip, host)
					}
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}

	client := &http.Client{
		Timeout:   opts.RequestTimeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &Dispatcher{
		webhooks:   webhooks,
		deliveries: deliveries,
		httpClient: client,
		opts:       opts,
	}
}

// DispatchAsync fans out an event to every active webhook subscribed to eventType.
// Returns immediately; all work happens in goroutines.
func (d *Dispatcher) DispatchAsync(eventID uuid.UUID, eventType string, data any) {
	go d.dispatch(eventID, eventType, data)
}

func (d *Dispatcher) dispatch(eventID uuid.UUID, eventType string, data any) {
	ctx := context.Background()
	hooks, err := d.webhooks.ListActiveForEvent(ctx, eventType)
	if err != nil {
		log.Printf("webhook dispatch: failed to list active webhooks for %s: %v", eventType, err)
		return
	}
	if len(hooks) == 0 {
		return
	}

	for _, w := range hooks {
		deliveryID := uuid.New()
		envelope := models.FormSubmissionPayload{
			ID:        deliveryID,
			Event:     eventType,
			CreatedAt: time.Now().UTC(),
			Data:      nil,
		}
		body, err := buildEnvelope(envelope, data)
		if err != nil {
			log.Printf("webhook dispatch: failed to marshal envelope: %v", err)
			continue
		}
		now := time.Now().UTC()
		dl := &models.WebhookDelivery{
			ID:        deliveryID,
			WebhookID: w.ID,
			EventType: eventType,
			EventID:   eventID,
			Payload:   json.RawMessage(body),
			Status:    models.DeliveryPending,
			Attempts:  0,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := d.deliveries.Create(ctx, dl); err != nil {
			log.Printf("webhook dispatch: failed to create delivery row: %v", err)
			continue
		}
		go d.executeAttempt(w, dl, body)
	}
}

// buildEnvelope marshals { id, event, createdAt, data } where data is the raw
// JSON of the supplied event payload.
func buildEnvelope(env models.FormSubmissionPayload, data any) ([]byte, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshaling event data: %w", err)
	}
	out := map[string]any{
		"id":        env.ID,
		"event":     env.Event,
		"createdAt": env.CreatedAt.Format(time.RFC3339Nano),
		"data":      json.RawMessage(dataBytes),
	}
	return json.Marshal(out)
}

// executeAttempt performs one HTTP attempt and either schedules the next or
// marks the delivery terminal.
func (d *Dispatcher) executeAttempt(w *models.Webhook, dl *models.WebhookDelivery, body []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), d.opts.RequestTimeout)
	defer cancel()

	attempt := dl.Attempts + 1
	statusCode, respBody, err := d.doRequest(ctx, w, dl, body)

	updateCtx := context.Background()

	if err == nil && statusCode >= 200 && statusCode < 300 {
		if uerr := d.deliveries.MarkSuccess(updateCtx, dl.ID, attempt, statusCode, respBody); uerr != nil {
			log.Printf("webhook delivery: failed to mark success for %s: %v", dl.ID, uerr)
		}
		log.Printf("webhook delivery success: webhook=%s delivery=%s attempt=%d status=%d", w.ID, dl.ID, attempt, statusCode)
		return
	}

	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	log.Printf("webhook delivery failed: webhook=%s delivery=%s attempt=%d status=%d err=%v", w.ID, dl.ID, attempt, statusCode, err)

	if attempt < maxAttempts {
		delay := d.opts.BackoffSchedule[attempt-1]
		nextAt := time.Now().Add(delay)
		if uerr := d.deliveries.MarkPendingRetry(updateCtx, dl.ID, attempt, statusCode, errMsg, respBody, nextAt); uerr != nil {
			log.Printf("webhook delivery: failed to mark pending retry for %s: %v", dl.ID, uerr)
		}
		dl.Attempts = attempt
		time.AfterFunc(delay, func() {
			d.executeAttempt(w, dl, body)
		})
		return
	}

	if uerr := d.deliveries.MarkFailed(updateCtx, dl.ID, attempt, statusCode, errMsg, respBody); uerr != nil {
		log.Printf("webhook delivery: failed to mark failed for %s: %v", dl.ID, uerr)
	}
}

// doRequest performs a single signed POST and returns status/body/err.
func (d *Dispatcher) doRequest(ctx context.Context, w *models.Webhook, dl *models.WebhookDelivery, body []byte) (int, string, error) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := signBody(w.Secret, ts, body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return 0, "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Webhook-Id", w.ID.String())
	req.Header.Set("X-Webhook-Delivery-Id", dl.ID.String())
	req.Header.Set("X-Webhook-Event", dl.EventType)
	req.Header.Set("X-Webhook-Timestamp", ts)
	req.Header.Set("X-Webhook-Signature", sig)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	return resp.StatusCode, string(raw), nil
}

// signBody returns "sha256=<hex>" where the digest is
// HMAC_SHA256(secret, timestamp + "." + body).
func signBody(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte{'.'})
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// validateWebhookURL enforces http/https + non-empty host. It does NOT resolve
// the host — that happens at dial time via the private-IP guard.
func validateWebhookURL(raw string) error {
	if raw == "" {
		return errors.New("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("url scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("url host is required")
	}
	if u.User != nil {
		return errors.New("url must not contain userinfo")
	}
	return nil
}

// isPrivateAddr returns true for IPs we refuse to dial when
// WEBHOOK_ALLOW_PRIVATE_IPS is false.
func isPrivateAddr(ip net.IP) bool {
	if ip == nil {
		return true // be conservative
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip.IsMulticast() {
		return true
	}
	// IsPrivate covers RFC1918 + RFC4193 (fc00::/7).
	if ip.IsPrivate() {
		return true
	}
	// Carrier-grade NAT 100.64.0.0/10 — not flagged by IsPrivate.
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1]&0xc0 == 64 {
			return true
		}
	}
	return false
}
