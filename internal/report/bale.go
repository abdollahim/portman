package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"portman/internal/logging"
)

// Bale posts messages with the Bale bot API, which uses the Telegram layout:
// POST {base}/bot{token}/sendMessage with chat_id and text.
type Bale struct {
	BaseURL  string
	Token    string
	ChatID   string
	HTTP     *http.Client
	Attempts int
	Backoff  time.Duration
}

// Send retries network errors and non-OK responses. Each retry waits longer.
// The HTTP client's errors include the URL, which contains the token. Logging
// redacts it, because the token is registered with logging.AddSecret.
func (b Bale) Send(text string) error {
	body, err := json.Marshal(map[string]string{"chat_id": b.ChatID, "text": text})
	if err != nil {
		return err
	}
	endpoint := b.BaseURL + "/bot" + b.Token + "/sendMessage"

	attempts := b.Attempts
	if attempts < 1 {
		attempts = 1
	}
	var last error
	for attempt := 1; attempt <= attempts; attempt++ {
		if last = b.post(endpoint, body); last == nil {
			return nil
		}
		logging.Logf("warn", "bale send attempt %d/%d failed: %v", attempt, attempts, last)
		if attempt < attempts {
			time.Sleep(time.Duration(attempt) * b.Backoff)
		}
	}
	return last
}

func (b Bale) post(endpoint string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var r struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(raw, &r)
	if resp.StatusCode != http.StatusOK || !r.OK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, r.Description)
	}
	return nil
}
