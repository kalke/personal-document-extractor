package groq

import (
	"bytes"
	"context"
	"encoding/json"
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
}

func New(apiKey, model, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *Client) Model() string {
	return c.model
}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL string `json:"url"`
}

type chatRequest struct {
	Model              string          `json:"model"`
	Messages           []Message       `json:"messages"`
	Temperature        float64         `json:"temperature"`
	MaxCompletionTokens int            `json:"max_completion_tokens,omitempty"`
	ReasoningEffort    string          `json:"reasoning_effort,omitempty"`
	ReasoningFormat    string          `json:"reasoning_format,omitempty"`
	ResponseFormat     *responseFormat `json:"response_format,omitempty"`
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

type ChatOpts struct {
	JSONMode bool
}

func (c *Client) Chat(ctx context.Context, messages []Message) (string, error) {
	return c.ChatWithOpts(ctx, messages, ChatOpts{JSONMode: false})
}

func (c *Client) ChatWithOpts(ctx context.Context, messages []Message, opts ChatOpts) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		content, err := c.chatOnce(ctx, messages, opts)
		if err == nil {
			return content, nil
		}
		lastErr = err
		if !isRateLimit(err) || attempt == 3 {
			return "", err
		}
		wait := time.Duration(attempt*3) * time.Second
		slog.Warn("groq rate limited; retrying", "attempt", attempt, "wait", wait.String(), "err", err)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(wait):
		}
	}
	return "", lastErr
}

func isRateLimit(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "rate limit") || strings.Contains(msg, "http 429")
}

func (c *Client) chatOnce(ctx context.Context, messages []Message, opts ChatOpts) (string, error) {
	start := time.Now()
	reqBody := chatRequest{
		Model:               c.model,
		Messages:            messages,
		Temperature:         0,
		MaxCompletionTokens: 2048,
		ReasoningEffort:     "none",
		ReasoningFormat:     "hidden",
	}
	if opts.JSONMode {
		reqBody.ResponseFormat = &responseFormat{Type: "json_object"}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	slog.Debug("groq request start", "model", c.model, "messages", len(messages), "json_mode", opts.JSONMode, "body_bytes", len(body))

	resp, err := c.http.Do(req)
	if err != nil {
		slog.Error("groq request failed", "err", err, "duration_ms", time.Since(start).Milliseconds())
		return "", fmt.Errorf("groq request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		slog.Error("groq decode failed", "status", resp.StatusCode, "body", truncate(string(raw), 200))
		return "", fmt.Errorf("decode groq response: %w (body=%s)", err, truncate(string(raw), 200))
	}
	if parsed.Error != nil {
		slog.Error("groq api error", "status", resp.StatusCode, "message", parsed.Error.Message, "duration_ms", time.Since(start).Milliseconds())
		return "", fmt.Errorf("groq error: %s", parsed.Error.Message)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Error("groq http error", "status", resp.StatusCode, "body", truncate(string(raw), 200), "duration_ms", time.Since(start).Milliseconds())
		return "", fmt.Errorf("groq http %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("groq returned no choices")
	}

	content, err := decodeMessageContent(parsed.Choices[0].Message.Content)
	if err != nil {
		return "", err
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
	var parts []ContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" {
				b.WriteString(p.Text)
			}
		}
		return b.String(), nil
	}
	return "", fmt.Errorf("unsupported groq content shape: %s", truncate(string(raw), 200))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
