package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rifat977/taplytix/internal/alert"
)

// Webhook posts JSON payloads describing fired alerts to a configured URL.
// Three retry attempts with constant 1s backoff; total per-attempt timeout 5s.
type Webhook struct {
	URL    string
	Client *http.Client
}

func NewWebhook(url string) *Webhook {
	return &Webhook{URL: url, Client: &http.Client{Timeout: 5 * time.Second}}
}

func (w *Webhook) Name() string { return "webhook" }

type webhookPayload struct {
	Alert     string  `json:"alert"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	FiredAt   string  `json:"fired_at"`
	Service   string  `json:"service"`
}

func (w *Webhook) Notify(ctx context.Context, a alert.Alert) error {
	if w.URL == "" {
		return errors.New("webhook: URL not configured")
	}
	body, err := json.Marshal(webhookPayload{
		Alert:     a.Rule.Name,
		Value:     a.Value,
		Threshold: a.Rule.Threshold,
		FiredAt:   a.FiredAt.UTC().Format(time.RFC3339),
		Service:   a.Service,
	})
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := w.Client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		lastErr = fmt.Errorf("webhook %s returned %d", w.URL, resp.StatusCode)
	}
	return lastErr
}
