package groq

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestChatStatusBeforeDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`not-json`))
	}))
	t.Cleanup(srv.Close)

	c := New("key", "model", srv.URL)
	c.http = srv.Client()

	_, err := c.Chat(context.Background(), "sys", "user", false)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("want HTTPError, got %v", err)
	}
	if httpErr.Status != http.StatusBadGateway {
		t.Fatalf("status=%d", httpErr.Status)
	}
}

func TestChatRetriesOn429(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	t.Cleanup(srv.Close)

	c := New("key", "model", srv.URL)
	c.http = &http.Client{Timeout: 5 * time.Second}
	c.wait = func(ctx context.Context, _ time.Duration) error {
		return ctx.Err()
	}

	got, err := c.Chat(context.Background(), "sys", "user", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Fatalf("got %q", got)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits=%d", hits.Load())
	}
}

func TestChatDoesNotRetryNon429(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	t.Cleanup(srv.Close)

	c := New("key", "model", srv.URL)
	c.http = srv.Client()

	_, err := c.Chat(context.Background(), "sys", "user", false)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.Status != http.StatusInternalServerError {
		t.Fatalf("got %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d want 1", hits.Load())
	}
}
