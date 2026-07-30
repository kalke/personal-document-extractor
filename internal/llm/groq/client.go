package groq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
	wait    func(ctx context.Context, d time.Duration) error
}

func New(apiKey, model, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 120 * time.Second,
		},
		wait: sleepContext,
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) Model() string {
	return c.model
}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type chatRequest struct {
	Model               string          `json:"model"`
	Messages            []Message       `json:"messages"`
	Temperature         float64         `json:"temperature"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	ReasoningEffort     string          `json:"reasoning_effort,omitempty"`
	ReasoningFormat     string          `json:"reasoning_format,omitempty"`
	ResponseFormat      *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("groq http %d: %s", e.Status, e.Body)
}

func (c *Client) Chat(ctx context.Context, system string, user any, jsonMode bool) (string, error) {
	msgs := []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		content, err := c.chatOnce(ctx, msgs, jsonMode)
		if err == nil {
			return content, nil
		}
		lastErr = err
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || httpErr.Status != http.StatusTooManyRequests || attempt == 3 {
			return "", err
		}
		wait := time.Duration(attempt*3) * time.Second
		slog.Warn("groq rate limited; retrying", "attempt", attempt, "wait", wait.String(), "err", err)
		if err := c.wait(ctx, wait); err != nil {
			return "", err
		}
	}
	return "", lastErr
}

func (c *Client) chatOnce(ctx context.Context, messages []Message, jsonMode bool) (string, error) {
	start := time.Now()
	reqBody := chatRequest{
		Model:               c.model,
		Messages:            messages,
		Temperature:         0,
		MaxCompletionTokens: 2048,
		ReasoningEffort:     "none",
		ReasoningFormat:     "hidden",
	}
	if jsonMode {
		reqBody.ResponseFormat = &responseFormat{Type: "json_object"}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal groq request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create groq request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	slog.Debug("groq request start", "model", c.model, "messages", len(messages), "json_mode", jsonMode, "body_bytes", len(body))

	resp, err := c.http.Do(req)
	if err != nil {
		slog.Error("groq request failed", "err", err, "duration_ms", time.Since(start).Milliseconds())
		return "", fmt.Errorf("groq request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read groq response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Error("groq http error", "status", resp.StatusCode, "body", truncate(string(raw)), "duration_ms", time.Since(start).Milliseconds())
		return "", &HTTPError{Status: resp.StatusCode, Body: truncate(string(raw))}
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		slog.Error("groq decode failed", "status", resp.StatusCode, "body", truncate(string(raw)))
		return "", fmt.Errorf("decode groq response: %w", err)
	}
	if parsed.Error != nil {
		slog.Error("groq api error", "status", resp.StatusCode, "message", parsed.Error.Message, "duration_ms", time.Since(start).Milliseconds())
		return "", fmt.Errorf("groq error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("groq returned no choices")
	}

	content, err := decodeMessageContent(parsed.Choices[0].Message.Content)
	if err != nil {
		return "", fmt.Errorf("decode groq content: %w", err)
	}

	slog.Debug("groq request ok",
		"status", resp.StatusCode,
		"duration_ms", time.Since(start).Milliseconds(),
		"reply_chars", len(content),
	)
	return content, nil
}

func decodeMessageContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("groq returned empty content")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" {
				b.WriteString(p.Text)
			}
		}
		return b.String(), nil
	}
	return "", fmt.Errorf("unsupported groq content shape: %s", truncate(string(raw)))
}

func truncate(s string) string {
	const n = 200
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
