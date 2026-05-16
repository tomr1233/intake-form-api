package services

import (
	"context"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tomr1233/intake-form-api/internal/models"
)

func TestSignBody(t *testing.T) {
	// Fixed inputs so a future refactor can't silently change the wire format.
	got := signBody("topsecret", "1700000000", []byte(`{"hello":"world"}`))
	// Pre-computed: hex(HMAC_SHA256("topsecret", "1700000000.{\"hello\":\"world\"}"))
	want := "sha256=79883357e4c4c4abee43cf4b32367d67a1344520479e3e8c85e98406a6d6a2a5"
	if got != want {
		t.Fatalf("signBody mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestIsPrivateAddr(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"1.2.3.4", false},
		{"8.8.8.8", false},
		{"127.0.0.1", true},
		{"127.255.255.255", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.32.0.1", false},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"0.0.0.0", true},
		{"100.64.0.1", true},
		{"::1", true},
		{"fc00::1", true},
		{"fe80::1", true},
		{"2001:db8::1", false},
		{"::ffff:127.0.0.1", true},
		{"::ffff:100.64.0.1", true},  // CGNAT mapped IPv6
		{"::ffff:1.2.3.4", false},    // public mapped IPv6
		{"224.0.0.1", true},          // multicast
		{"239.255.255.255", true},    // multicast
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("parse failed for %q", tc.ip)
			}
			if got := isPrivateAddr(ip); got != tc.want {
				t.Fatalf("isPrivateAddr(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestValidateWebhookURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"plain http", "http://example.com/hook", false},
		{"https with port", "https://example.com:8443/hook", false},
		{"missing scheme", "example.com/hook", true},
		{"ftp scheme", "ftp://example.com", true},
		{"no host", "http:///hook", true},
		{"empty", "", true},
		{"malformed", "://bad", true},
		{"with userinfo", "http://user:pass@example.com/hook", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWebhookURL(tc.url)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// --- in-memory repository fakes -------------------------------------------------

type fakeWebhookRepo struct {
	mu       sync.Mutex
	webhooks map[uuid.UUID]*models.Webhook
}

func newFakeWebhookRepo() *fakeWebhookRepo {
	return &fakeWebhookRepo{webhooks: map[uuid.UUID]*models.Webhook{}}
}

func (r *fakeWebhookRepo) add(w *models.Webhook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.webhooks[w.ID] = w
}

func (r *fakeWebhookRepo) ListActiveForEvent(_ context.Context, eventType string) ([]*models.Webhook, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*models.Webhook
	for _, w := range r.webhooks {
		if !w.Active {
			continue
		}
		for _, e := range w.Events {
			if e == eventType {
				out = append(out, w)
				break
			}
		}
	}
	return out, nil
}

type fakeDeliveryRepo struct {
	mu         sync.Mutex
	deliveries map[uuid.UUID]*models.WebhookDelivery
}

func newFakeDeliveryRepo() *fakeDeliveryRepo {
	return &fakeDeliveryRepo{deliveries: map[uuid.UUID]*models.WebhookDelivery{}}
}

func (r *fakeDeliveryRepo) Create(_ context.Context, d *models.WebhookDelivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := *d
	r.deliveries[d.ID] = &c
	return nil
}

func (r *fakeDeliveryRepo) get(id uuid.UUID) *models.WebhookDelivery {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.deliveries[id]
	if !ok {
		return nil
	}
	c := *d
	return &c
}

func (r *fakeDeliveryRepo) all() []*models.WebhookDelivery {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*models.WebhookDelivery, 0, len(r.deliveries))
	for _, d := range r.deliveries {
		c := *d
		out = append(out, &c)
	}
	return out
}

func (r *fakeDeliveryRepo) MarkSuccess(_ context.Context, id uuid.UUID, attempts int, statusCode int, respBody string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.deliveries[id]
	if d == nil {
		return nil
	}
	d.Status = models.DeliverySuccess
	d.Attempts = attempts
	sc := statusCode
	d.LastStatusCode = &sc
	rb := respBody
	d.LastResponseBody = &rb
	d.LastError = nil
	d.NextAttemptAt = nil
	d.UpdatedAt = time.Now()
	return nil
}

func (r *fakeDeliveryRepo) MarkPendingRetry(_ context.Context, id uuid.UUID, attempts int, statusCode int, errMsg, respBody string, nextAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.deliveries[id]
	if d == nil {
		return nil
	}
	d.Status = models.DeliveryPending
	d.Attempts = attempts
	if statusCode != 0 {
		sc := statusCode
		d.LastStatusCode = &sc
	}
	em := errMsg
	d.LastError = &em
	rb := respBody
	d.LastResponseBody = &rb
	na := nextAt
	d.NextAttemptAt = &na
	d.UpdatedAt = time.Now()
	return nil
}

func (r *fakeDeliveryRepo) MarkFailed(_ context.Context, id uuid.UUID, attempts int, statusCode int, errMsg, respBody string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.deliveries[id]
	if d == nil {
		return nil
	}
	d.Status = models.DeliveryFailed
	d.Attempts = attempts
	if statusCode != 0 {
		sc := statusCode
		d.LastStatusCode = &sc
	}
	em := errMsg
	d.LastError = &em
	rb := respBody
	d.LastResponseBody = &rb
	d.NextAttemptAt = nil
	d.UpdatedAt = time.Now()
	return nil
}

// --- helpers --------------------------------------------------------------------

func newTestDispatcher(t *testing.T, wr *fakeWebhookRepo, dr *fakeDeliveryRepo, allowPrivate bool) *Dispatcher {
	t.Helper()
	return NewDispatcher(wr, dr, DispatcherOptions{
		AllowPrivateIPs: allowPrivate,
		BackoffSchedule: []time.Duration{1 * time.Millisecond, 1 * time.Millisecond, 1 * time.Millisecond},
		RequestTimeout:  2 * time.Second,
	})
}

func waitForTerminal(t *testing.T, dr *fakeDeliveryRepo, webhookID uuid.UUID, timeout time.Duration) *models.WebhookDelivery {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, d := range dr.all() {
			if d.WebhookID == webhookID && (d.Status == models.DeliverySuccess || d.Status == models.DeliveryFailed) {
				return d
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no terminal delivery for webhook %s within %s", webhookID, timeout)
	return nil
}

func newWebhook(t *testing.T, url string, active bool) *models.Webhook {
	t.Helper()
	return &models.Webhook{
		ID:           uuid.New(),
		Name:         "test",
		URL:          url,
		Secret:       "topsecret",
		SecretPrefix: "topsecre",
		Events:       []string{models.EventTypeFormSubmission},
		Active:       active,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

func sampleSubmission() *models.Submission {
	return &models.Submission{
		ID:        uuid.New(),
		Status:    models.StatusPending,
		FirstName: "Test",
		Email:     "test@example.com",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// --- the actual tests -----------------------------------------------------------

func TestDispatch_HappyPath(t *testing.T) {
	var got struct {
		mu     sync.Mutex
		sig    string
		ts     string
		body   []byte
		called int32
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.mu.Lock()
		got.sig = r.Header.Get("X-Webhook-Signature")
		got.ts = r.Header.Get("X-Webhook-Timestamp")
		body, _ := io.ReadAll(r.Body)
		got.body = body
		got.mu.Unlock()
		atomic.AddInt32(&got.called, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wr := newFakeWebhookRepo()
	dr := newFakeDeliveryRepo()
	w := newWebhook(t, srv.URL, true)
	wr.add(w)

	d := newTestDispatcher(t, wr, dr, true)
	sub := sampleSubmission()
	d.DispatchAsync(sub.ID, models.EventTypeFormSubmission, sub)

	got2 := waitForTerminal(t, dr, w.ID, 3*time.Second)
	if got2.Status != models.DeliverySuccess {
		t.Fatalf("status = %s, want success", got2.Status)
	}
	if got2.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", got2.Attempts)
	}
	if atomic.LoadInt32(&got.called) != 1 {
		t.Fatalf("server called %d times, want 1", got.called)
	}

	got.mu.Lock()
	expected := signBody(w.Secret, got.ts, got.body)
	got.mu.Unlock()
	if got.sig != expected {
		t.Fatalf("signature mismatch:\n got: %s\nwant: %s", got.sig, expected)
	}

	parsed, err := strconv.ParseInt(got.ts, 10, 64)
	if err != nil {
		t.Fatalf("timestamp not an int64: %v", err)
	}
	if diff := time.Since(time.Unix(parsed, 0)); diff > 5*time.Second || diff < -5*time.Second {
		t.Fatalf("timestamp drift too large: %s", diff)
	}

	if !strings.HasPrefix(got.sig, "sha256=") || len(got.sig) != len("sha256=")+64 {
		t.Fatalf("signature shape wrong: %q", got.sig)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(got.sig, "sha256=")); err != nil {
		t.Fatalf("signature hex invalid: %v", err)
	}
}

func TestDispatch_RetryThenSucceed(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wr := newFakeWebhookRepo()
	dr := newFakeDeliveryRepo()
	w := newWebhook(t, srv.URL, true)
	wr.add(w)

	d := newTestDispatcher(t, wr, dr, true)
	sub := sampleSubmission()
	d.DispatchAsync(sub.ID, models.EventTypeFormSubmission, sub)

	got := waitForTerminal(t, dr, w.ID, 3*time.Second)
	if got.Status != models.DeliverySuccess {
		t.Fatalf("status = %s, want success", got.Status)
	}
	if got.Attempts != 3 {
		t.Fatalf("attempts = %d, want 3", got.Attempts)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("server called %d times, want 3", calls)
	}
}

func TestDispatch_PermanentFailure(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	wr := newFakeWebhookRepo()
	dr := newFakeDeliveryRepo()
	w := newWebhook(t, srv.URL, true)
	wr.add(w)

	d := newTestDispatcher(t, wr, dr, true)
	sub := sampleSubmission()
	d.DispatchAsync(sub.ID, models.EventTypeFormSubmission, sub)

	got := waitForTerminal(t, dr, w.ID, 3*time.Second)
	if got.Status != models.DeliveryFailed {
		t.Fatalf("status = %s, want failed", got.Status)
	}
	if got.Attempts != 4 {
		t.Fatalf("attempts = %d, want 4", got.Attempts)
	}
	if got.LastStatusCode == nil || *got.LastStatusCode != 500 {
		t.Fatalf("last status code = %v, want 500", got.LastStatusCode)
	}
	if atomic.LoadInt32(&calls) != 4 {
		t.Fatalf("server called %d times, want 4", calls)
	}
}

func TestDispatch_DisabledWebhookSkipped(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wr := newFakeWebhookRepo()
	dr := newFakeDeliveryRepo()
	w := newWebhook(t, srv.URL, false)
	wr.add(w)

	d := newTestDispatcher(t, wr, dr, true)
	sub := sampleSubmission()
	d.DispatchAsync(sub.ID, models.EventTypeFormSubmission, sub)

	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("server called %d times, want 0", calls)
	}
	if len(dr.all()) != 0 {
		t.Fatalf("delivery rows created for disabled webhook: %d", len(dr.all()))
	}
}

func TestDispatch_SSRFBlocked(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wr := newFakeWebhookRepo()
	dr := newFakeDeliveryRepo()
	w := newWebhook(t, srv.URL, true)
	wr.add(w)

	d := newTestDispatcher(t, wr, dr, false)
	sub := sampleSubmission()
	d.DispatchAsync(sub.ID, models.EventTypeFormSubmission, sub)

	got := waitForTerminal(t, dr, w.ID, 3*time.Second)
	if got.Status != models.DeliveryFailed {
		t.Fatalf("status = %s, want failed", got.Status)
	}
	if got.LastError == nil || !strings.Contains(*got.LastError, "private") {
		t.Fatalf("expected private-IP error, got: %v", got.LastError)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("server called %d times despite block, want 0", calls)
	}
}
