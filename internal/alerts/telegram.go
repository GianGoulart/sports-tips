package alerts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TelegramAlerter sends messages via the Telegram Bot API.
type TelegramAlerter struct {
	botToken   string
	httpClient *http.Client
}

func NewTelegramAlerter(botToken string) *TelegramAlerter {
	return &TelegramAlerter{
		botToken:   botToken,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Send sends a plain-text message to the given chat ID.
// chatID is the Telegram user or group ID stored in tenant_preferences.alert_telegram_id.
func (t *TelegramAlerter) Send(chatID, message string) error {
	if t.botToken == "" || chatID == "" {
		return nil // silently skip if not configured
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)

	body, err := json.Marshal(map[string]string{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "HTML",
	})
	if err != nil {
		return fmt.Errorf("telegram marshal: %w", err)
	}

	resp, err := t.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram post: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram status %d", resp.StatusCode)
	}

	return nil
}

// NoopAlerter is a no-op alerter used when bot token is empty.
type NoopAlerter struct{}

func (n *NoopAlerter) Send(chatID, message string) error { return nil }

// Alerter is the interface both TelegramAlerter and NoopAlerter satisfy.
type Alerter interface {
	Send(chatID, message string) error
}
