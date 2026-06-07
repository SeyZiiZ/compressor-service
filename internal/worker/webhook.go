package worker

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// WebhookPayload is the JSON body POSTed to a job's webhook_url on completion/failure.
type WebhookPayload struct {
	JobID            string  `json:"job_id"`
	Status           string  `json:"status"` // "completed" | "failed"
	OutputKey        string  `json:"output_key,omitempty"`
	MimeType         string  `json:"mime_type,omitempty"`
	OutputSize       int64   `json:"output_size,omitempty"`
	CompressionRatio float64 `json:"compression_ratio,omitempty"`
	Error            string  `json:"error,omitempty"`
}

// WebhookClient delivers signed completion callbacks to the calling backend.
type WebhookClient struct {
	http   *http.Client
	secret string
}

func NewWebhookClient(secret string, timeoutSeconds int) *WebhookClient {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 15
	}
	return &WebhookClient{
		http:   &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
		secret: secret,
	}
}

// Send delivers a signed callback. Safe to run in a goroutine: it owns its
// timeout, retries up to 3×, and never panics. Signature scheme (verified by the
// backend): X-Compressor-Signature: sha256=hex(HMAC_SHA256(secret, timestamp + "." + body)),
// X-Compressor-Timestamp: unix seconds.
func (c *WebhookClient) Send(url string, payload WebhookPayload) {
	if url == "" {
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("webhook: marshal failed for job %s: %v", payload.JobID, err)
		return
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := c.sign(body, ts)

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			log.Printf("webhook: bad request for job %s: %v", payload.JobID, err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Compressor-Timestamp", ts)
		req.Header.Set("X-Compressor-Signature", "sha256="+sig)

		resp, err := c.http.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	log.Printf("webhook: delivery failed for job %s after retries: %v", payload.JobID, lastErr)
}

func (c *WebhookClient) sign(body []byte, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(c.secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
