package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/kalke/personal-document-extractor/internal/auth"
	"github.com/kalke/personal-document-extractor/internal/cache"
	"github.com/kalke/personal-document-extractor/internal/extract"
	"github.com/kalke/personal-document-extractor/internal/httpapi"
	"github.com/kalke/personal-document-extractor/internal/preprocess"
	"github.com/kalke/personal-document-extractor/internal/ratelimit"
	"github.com/kalke/personal-document-extractor/internal/store"
)

const testBearer = "test-token"

type stubAuth struct {
	principals map[string]auth.Principal
	err        error
}

func (s *stubAuth) Authenticate(_ context.Context, token string) (auth.Principal, error) {
	if s.err != nil {
		return auth.Principal{}, s.err
	}
	if p, ok := s.principals[token]; ok {
		return p, nil
	}
	return auth.Principal{}, auth.ErrUnauthorized
}

type allowAllLimiter struct{}

func (allowAllLimiter) Allow(context.Context, string, string) (ratelimit.Result, error) {
	return ratelimit.Result{Allowed: true, Limit: 1000, Remaining: 999}, nil
}

func defaultTestAuth() *stubAuth {
	return &stubAuth{
		principals: map[string]auth.Principal{
			testBearer: {
				Subject:  "api_key:test",
				Kind:     auth.KindAPIKey,
				APIKeyID: "00000000-0000-0000-0000-000000000001",
				Scopes:   []string{auth.ScopeExtractWrite},
			},
		},
	}
}

type stubExtractor struct {
	mu     sync.Mutex
	types  []string
	result extract.Result
	err    error
	calls  int
}

func (s *stubExtractor) KnownTypes() []string { return s.types }

func (s *stubExtractor) Extract(_ context.Context, _ string, _ preprocess.PreparedDocument) (extract.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.result, s.err
}

func (s *stubExtractor) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type stubDB struct{ err error }

func (s stubDB) Ping(context.Context) error { return s.err }

type memoryStore struct {
	mu       sync.Mutex
	rows     []store.ExtractionRecord
	replaced int
	err      error
}

func (m *memoryStore) Insert(_ context.Context, rec store.ExtractionRecord) error {
	return m.save(rec, false)
}

func (m *memoryStore) Replace(_ context.Context, rec store.ExtractionRecord) error {
	return m.save(rec, true)
}

func (m *memoryStore) save(rec store.ExtractionRecord, replace bool) error {
	if m.err != nil {
		return m.err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if replace {
		m.replaced++
	}
	m.rows = append(m.rows, rec)
	return nil
}

func (m *memoryStore) len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rows)
}

func (m *memoryStore) replaceCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.replaced
}

func mustHandler(t *testing.T, deps httpapi.Deps) http.Handler {
	t.Helper()
	if deps.Auth == nil {
		deps.Auth = defaultTestAuth()
	}
	if deps.RateLimit == nil {
		deps.RateLimit = allowAllLimiter{}
	}
	h, err := httpapi.New(deps)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestNewRequiresDeps(t *testing.T) {
	if _, err := httpapi.New(httpapi.Deps{}); err == nil {
		t.Fatal("expected error for missing extractor")
	}
	if _, err := httpapi.New(httpapi.Deps{Extractor: &stubExtractor{}}); err == nil {
		t.Fatal("expected error for missing auth")
	}
	if _, err := httpapi.New(httpapi.Deps{Extractor: &stubExtractor{}, Auth: defaultTestAuth()}); err == nil {
		t.Fatal("expected error for missing rate limiter")
	}
}

func TestExtractUnauthorized(t *testing.T) {
	h := mustHandler(t, httpapi.Deps{Extractor: &stubExtractor{}})
	rec := httptest.NewRecorder()
	req := multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", tinyPNG(t))
	req.Header.Del("Authorization")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestExtractForbiddenScope(t *testing.T) {
	h := mustHandler(t, httpapi.Deps{
		Extractor: &stubExtractor{},
		Auth: &stubAuth{principals: map[string]auth.Principal{
			testBearer: {Subject: "x", Kind: auth.KindAPIKey, Scopes: []string{auth.ScopeKeysManage}},
		}},
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", tinyPNG(t)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestExtractAdminScopeAllowed(t *testing.T) {
	h := mustHandler(t, httpapi.Deps{
		Extractor: &stubExtractor{
			result: extract.Result{
				DocType: "identity_document",
				Data:    map[string]any{"nome": "ADMIN"},
				Meta:    extract.Meta{Mode: "vision"},
			},
		},
		Auth: &stubAuth{principals: map[string]auth.Principal{
			testBearer: {
				Subject:  "api_key:admin",
				Kind:     auth.KindAPIKey,
				APIKeyID: "00000000-0000-0000-0000-000000000099",
				Scopes:   []string{auth.ScopeAdmin},
			},
		}},
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", tinyPNG(t)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

type countingLimiter struct {
	limit int
	n     int
}

func (c *countingLimiter) Allow(context.Context, string, string) (ratelimit.Result, error) {
	c.n++
	remaining := c.limit - c.n
	if remaining < 0 {
		remaining = 0
	}
	return ratelimit.Result{
		Allowed:    c.n <= c.limit,
		Limit:      c.limit,
		Remaining:  remaining,
		RetryAfter: time.Second,
	}, nil
}

func TestExtractRateLimited(t *testing.T) {
	lim := &countingLimiter{limit: 1}
	h := mustHandler(t, httpapi.Deps{
		Extractor: &stubExtractor{
			result: extract.Result{
				DocType: "identity_document",
				Data:    map[string]any{"nome": "X"},
				Meta:    extract.Meta{Mode: "vision"},
			},
		},
		RateLimit: lim,
	})
	png := tinyPNG(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", png))
	if rec.Code != http.StatusOK {
		t.Fatalf("first status %d", rec.Code)
	}
	if rec.Header().Get("X-RateLimit-Limit") == "" {
		t.Fatal("missing rate limit header")
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", png))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second status %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
}

func TestHealth(t *testing.T) {
	h := mustHandler(t, httpapi.Deps{
		Extractor: &stubExtractor{types: []string{"identity_document"}},
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestReadyRequiresDB(t *testing.T) {
	h := mustHandler(t, httpapi.Deps{
		Extractor: &stubExtractor{},
		Pool:      stubDB{err: errors.New("down")},
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}

	h = mustHandler(t, httpapi.Deps{
		Extractor: &stubExtractor{},
		Pool:      stubDB{},
	})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestExtractValidation(t *testing.T) {
	h := mustHandler(t, httpapi.Deps{Extractor: &stubExtractor{}})

	t.Run("missing doc_type", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, multipartRequest(t, "/v1/extract", "doc.png", tinyPNG(t)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status %d", rec.Code)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/extract?doc_type=identity_document", nil)
		req.Header.Set("Authorization", "Bearer "+testBearer)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status %d", rec.Code)
		}
	})
}

func TestExtractCacheHitAndMiss(t *testing.T) {
	mr := miniredis.RunT(t)
	c := cache.New(mr.Addr(), "", 0, time.Hour)
	t.Cleanup(func() { _ = c.Close() })

	st := &memoryStore{}
	ex := &stubExtractor{
		types: []string{"identity_document"},
		result: extract.Result{
			DocType: "identity_document",
			Data:    map[string]any{"nome": "FULANO DA SILVA"},
			Meta:    extract.Meta{Model: "stub", Mode: "vision", Images: 1},
		},
	}
	h := mustHandler(t, httpapi.Deps{
		Extractor:   ex,
		Pool:        stubDB{},
		Cache:       c,
		Extractions: st,
	})

	png := tinyPNG(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", png))
	if rec.Code != http.StatusOK {
		t.Fatalf("miss status %d body %s", rec.Code, rec.Body.String())
	}
	var miss map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &miss); err != nil {
		t.Fatal(err)
	}
	if miss["doc_type"] != "identity_document" {
		t.Fatalf("miss body: %v", miss)
	}
	if _, ok := miss["meta"]; ok {
		t.Fatalf("meta must not be exposed: %v", miss)
	}
	if ex.callCount() != 1 || st.len() != 1 {
		t.Fatalf("calls=%d rows=%d", ex.callCount(), st.len())
	}
	if st.rows[0].ClientIP == "" {
		t.Fatal("expected client_ip persisted")
	}
	if st.rows[0].AuthSubject == "" || st.rows[0].APIKeyID == "" {
		t.Fatalf("expected principal persisted: %+v", st.rows[0])
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", png))
	if rec.Code != http.StatusOK {
		t.Fatalf("hit status %d", rec.Code)
	}
	var hit map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &hit); err != nil {
		t.Fatal(err)
	}
	if _, ok := hit["meta"]; ok {
		t.Fatalf("meta must not be exposed on hit: %v", hit)
	}
	if ex.callCount() != 1 || st.len() != 1 {
		t.Fatalf("cache hit should not extract/persist again; calls=%d rows=%d", ex.callCount(), st.len())
	}
}

func TestExtractValidatesBeforeCache(t *testing.T) {
	mr := miniredis.RunT(t)
	c := cache.New(mr.Addr(), "", 0, time.Hour)
	t.Cleanup(func() { _ = c.Close() })

	ex := &stubExtractor{
		result: extract.Result{
			DocType: "identity_document",
			Data:    map[string]any{"nome": "FULANO DA SILVA"},
			Meta:    extract.Meta{Mode: "vision"},
		},
	}
	h := mustHandler(t, httpapi.Deps{Extractor: ex, Cache: c})

	png := tinyPNG(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", png))
	if rec.Code != http.StatusOK {
		t.Fatalf("seed status %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "photo.pdf", png))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected mismatch 400, got %d body %s", rec.Code, rec.Body.String())
	}
	if ex.callCount() != 1 {
		t.Fatalf("spoofed extension must not use cache hit; calls=%d", ex.callCount())
	}
}

func TestExtractRefreshBypassesCacheAndReplaces(t *testing.T) {
	mr := miniredis.RunT(t)
	c := cache.New(mr.Addr(), "", 0, time.Hour)
	t.Cleanup(func() { _ = c.Close() })

	st := &memoryStore{}
	ex := &stubExtractor{
		result: extract.Result{
			DocType: "identity_document",
			Data:    map[string]any{"nome": "FULANO DA SILVA"},
			Meta:    extract.Meta{Mode: "vision"},
		},
	}
	h := mustHandler(t, httpapi.Deps{Extractor: ex, Cache: c, Extractions: st})
	png := tinyPNG(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", png))
	if rec.Code != http.StatusOK || ex.callCount() != 1 {
		t.Fatalf("seed status=%d calls=%d", rec.Code, ex.callCount())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", png))
	if ex.callCount() != 1 {
		t.Fatalf("expected cache hit; calls=%d", ex.callCount())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document&refresh=true", "doc.png", png))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status %d body %s", rec.Code, rec.Body.String())
	}
	if ex.callCount() != 2 {
		t.Fatalf("refresh should re-extract; calls=%d", ex.callCount())
	}
	if st.len() != 2 || st.replaceCount() != 1 {
		t.Fatalf("rows=%d replaced=%d", st.len(), st.replaceCount())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["meta"]; ok {
		t.Fatalf("meta must not be exposed: %v", body)
	}
}

func TestExtractStoreFailureStillOK(t *testing.T) {
	ex := &stubExtractor{
		result: extract.Result{
			DocType: "identity_document",
			Data:    map[string]any{"nome": "FULANO DA SILVA"},
			Meta:    extract.Meta{Mode: "vision"},
		},
	}
	h := mustHandler(t, httpapi.Deps{
		Extractor:   ex,
		Extractions: &memoryStore{err: errors.New("db down")},
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", tinyPNG(t)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestExtractMapsErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
		msg  string
	}{
		{"unknown", extract.ErrUnknownDocType, http.StatusBadRequest, "unknown doc_type"},
		{"bad_json", extract.ErrInvalidJSON, http.StatusUnprocessableEntity, "could not process document"},
		{"llm", extract.ErrLLM, http.StatusBadGateway, "extraction provider unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := mustHandler(t, httpapi.Deps{
				Extractor: &stubExtractor{err: tc.err},
			})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", tinyPNG(t)))
			if rec.Code != tc.code {
				t.Fatalf("status %d want %d body %s", rec.Code, tc.code, rec.Body.String())
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["error"] != tc.msg {
				t.Fatalf("error %q want %q", body["error"], tc.msg)
			}
		})
	}
}

func multipartRequest(t *testing.T, url, filename string, data []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, url, &buf)
	req.RemoteAddr = "203.0.113.10:54321"
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("User-Agent", "extract-test/1.0")
	req.Header.Set("Authorization", "Bearer "+testBearer)
	return req
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
