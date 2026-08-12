package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

const testAdminEmail = "henriquekalke@icloud.com"

func defaultTestAuth() *stubAuth {
	return &stubAuth{
		principals: map[string]auth.Principal{
			testBearer: {
				Subject: "oidc|test",
				Client:  "kalke-cli",
				Email:   testAdminEmail,
				Kind:    auth.KindJWT,
				Scopes:  []string{auth.ScopeAdmin},
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
	rows     []memoryRow
	replaced int
	err      error
	seq      int
}

type memoryRow struct {
	id  string
	rec store.ExtractionRecord
}

func (m *memoryStore) Insert(_ context.Context, rec store.ExtractionRecord) error {
	return m.save(rec, false)
}

func (m *memoryStore) Replace(_ context.Context, rec store.ExtractionRecord) error {
	return m.save(rec, true)
}

func (m *memoryStore) ListBySubject(_ context.Context, subject, docType string, limit int) ([]store.ExtractionSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]store.ExtractionSummary, 0)
	for i := len(m.rows) - 1; i >= 0; i-- {
		row := m.rows[i]
		if row.rec.AuthSubject != subject {
			continue
		}
		if docType != "" && row.rec.DocType != docType {
			continue
		}
		payload, _ := json.Marshal(row.rec.Result)
		out = append(out, store.ExtractionSummary{
			ID:            row.id,
			CreatedAt:     time.Now().UTC(),
			DocType:       row.rec.DocType,
			Filename:      row.rec.Filename,
			ContentSHA256: row.rec.ContentSHA256,
			Status:        row.rec.Status,
			Result:        payload,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memoryStore) GetByIDForSubject(_ context.Context, id, subject string) (store.ExtractionSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.rows {
		if row.id != id {
			continue
		}
		if row.rec.AuthSubject != subject {
			return store.ExtractionSummary{}, store.ErrNotFound
		}
		payload, _ := json.Marshal(row.rec.Result)
		return store.ExtractionSummary{
			ID:            row.id,
			CreatedAt:     time.Now().UTC(),
			DocType:       row.rec.DocType,
			Filename:      row.rec.Filename,
			ContentSHA256: row.rec.ContentSHA256,
			Status:        row.rec.Status,
			Result:        payload,
		}, nil
	}
	return store.ExtractionSummary{}, store.ErrNotFound
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
	m.seq++
	m.rows = append(m.rows, memoryRow{
		id:  fmt.Sprintf("mem-%d", m.seq),
		rec: rec,
	})
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

type memoryConsentStore struct {
	mu   sync.Mutex
	rows []store.ConsentRecord
	err  error
}

func (m *memoryConsentStore) Insert(_ context.Context, rec store.ConsentRecord) error {
	if m.err != nil {
		return m.err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows = append(m.rows, rec)
	return nil
}

func (m *memoryConsentStore) len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rows)
}

func mustHandler(t *testing.T, deps httpapi.Deps) http.Handler {
	t.Helper()
	if deps.Auth == nil {
		deps.Auth = defaultTestAuth()
	}
	if deps.RateLimit == nil {
		deps.RateLimit = allowAllLimiter{}
	}
	if deps.Consents == nil {
		deps.Consents = &memoryConsentStore{}
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
	if _, err := httpapi.New(httpapi.Deps{
		Extractor: &stubExtractor{},
		Auth:      defaultTestAuth(),
		RateLimit: allowAllLimiter{},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
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

func TestExtractAnyAuthenticatedUserAllowed(t *testing.T) {
	consents := &memoryConsentStore{}
	h := mustHandler(t, httpapi.Deps{
		Extractor: &stubExtractor{
			result: extract.Result{
				DocType: "identity_document",
				Data:    map[string]any{"nome": "USER"},
				Meta:    extract.Meta{Mode: "vision"},
			},
		},
		Consents: consents,
		Auth: &stubAuth{principals: map[string]auth.Principal{
			testBearer: {
				Subject: "oidc|signup-user",
				Email:   "recruiter@example.com",
				Kind:    auth.KindJWT,
				Scopes:  []string{"openid"},
			},
		}},
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", tinyPNG(t)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if consents.len() != 0 {
		t.Fatalf("expected no consent row, got %d", consents.len())
	}
}

func TestExtractWriteScopeAllowed(t *testing.T) {
	consents := &memoryConsentStore{}
	h := mustHandler(t, httpapi.Deps{
		Extractor: &stubExtractor{
			result: extract.Result{
				DocType: "identity_document",
				Data:    map[string]any{"nome": "USER"},
				Meta:    extract.Meta{Mode: "vision"},
			},
		},
		Consents: consents,
		Auth: &stubAuth{principals: map[string]auth.Principal{
			testBearer: {
				Subject: "oidc|signup-user",
				Email:   "recruiter@example.com",
				Kind:    auth.KindJWT,
				Scopes:  []string{auth.ScopeExtractWrite},
			},
		}},
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", tinyPNG(t)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if consents.len() != 1 {
		t.Fatalf("expected consent row, got %d", consents.len())
	}
	if consents.rows[0].PolicyVersion != store.PolicyLGPDExtractV1 {
		t.Fatalf("policy %q", consents.rows[0].PolicyVersion)
	}
}

func TestExtractConsentRequired(t *testing.T) {
	h := mustHandler(t, httpapi.Deps{
		Extractor: &stubExtractor{
			result: extract.Result{
				DocType: "identity_document",
				Data:    map[string]any{"nome": "X"},
				Meta:    extract.Meta{Mode: "vision"},
			},
		},
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequestNoConsent(t, "/v1/extract?doc_type=identity_document", "doc.png", tinyPNG(t)))
	if rec.Code != http.StatusBadRequest {
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
	// Different image bytes → cache miss → second LLM attempt is rate limited.
	png2 := tinyPNG(t)
	png2[len(png2)/2] ^= 0x01
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc2.png", png2))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second status %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
}

func TestExtractCacheHitSkipsRateLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	c := cache.New(mr.Addr(), "", 0, time.Hour, false)
	t.Cleanup(func() { _ = c.Close() })

	lim := &countingLimiter{limit: 1}
	ex := &stubExtractor{
		result: extract.Result{
			DocType: "identity_document",
			Data:    map[string]any{"nome": "X"},
			Meta:    extract.Meta{Mode: "vision"},
		},
	}
	h := mustHandler(t, httpapi.Deps{
		Extractor:   ex,
		Cache:       c,
		Extractions: &memoryStore{},
		RateLimit:   lim,
	})
	png := tinyPNG(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", png))
	if rec.Code != http.StatusOK {
		t.Fatalf("miss status %d", rec.Code)
	}
	if lim.n != 1 {
		t.Fatalf("limiter calls on miss: %d", lim.n)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, multipartRequest(t, "/v1/extract?doc_type=identity_document", "doc.png", png))
	if rec.Code != http.StatusOK {
		t.Fatalf("hit status %d body %s", rec.Code, rec.Body.String())
	}
	if lim.n != 1 {
		t.Fatalf("cache hit must not consume rate limit; got %d", lim.n)
	}
	if rec.Header().Get("X-RateLimit-Limit") != "" {
		t.Fatal("cache hit should not set rate limit headers")
	}
	if ex.callCount() != 1 {
		t.Fatalf("extractor calls=%d", ex.callCount())
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
	c := cache.New(mr.Addr(), "", 0, time.Hour, false)
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
	if st.rows[0].rec.ClientIP == "" {
		t.Fatal("expected client_ip persisted")
	}
	if st.rows[0].rec.AuthSubject == "" {
		t.Fatalf("expected auth_subject persisted: %+v", st.rows[0].rec)
	}
	if st.rows[0].rec.AuthClient != "kalke-cli" || st.rows[0].rec.AuthEmail != testAdminEmail {
		t.Fatalf("expected auth_client/email persisted: %+v", st.rows[0].rec)
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
	if ex.callCount() != 1 || st.len() != 2 {
		t.Fatalf("cache hit should not re-extract but still persist for the actor; calls=%d rows=%d", ex.callCount(), st.len())
	}
}

func TestExtractValidatesBeforeCache(t *testing.T) {
	mr := miniredis.RunT(t)
	c := cache.New(mr.Addr(), "", 0, time.Hour, false)
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
	c := cache.New(mr.Addr(), "", 0, time.Hour, false)
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
	if st.len() != 3 || st.replaceCount() != 1 {
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
	return multipartRequestOpt(t, url, filename, data, true)
}

func multipartRequestNoConsent(t *testing.T, url, filename string, data []byte) *http.Request {
	t.Helper()
	return multipartRequestOpt(t, url, filename, data, false)
}

func multipartRequestOpt(t *testing.T, url, filename string, data []byte, withConsent bool) *http.Request {
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
	if withConsent {
		if err := w.WriteField("consent", store.PolicyLGPDExtractV1); err != nil {
			t.Fatal(err)
		}
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

func TestExtractionsIDORBlockedBetweenUsers(t *testing.T) {
	const (
		tokenA = "token-a"
		tokenB = "token-b"
		subA   = "user-a"
		subB   = "user-b"
	)
	st := &memoryStore{}
	_ = st.Insert(context.Background(), store.ExtractionRecord{
		DocType:       "curriculum_vitae",
		ContentSHA256: "abc",
		Filename:      "a.pdf",
		AuthSubject:   subA,
		Status:        "success",
		Result: extract.Result{
			DocType: "curriculum_vitae",
			Data:    map[string]any{"full_name": "Secret Alice"},
		},
	})
	ownerID := st.rows[0].id

	h := mustHandler(t, httpapi.Deps{
		Extractor:   &stubExtractor{},
		Extractions: st,
		Auth: &stubAuth{principals: map[string]auth.Principal{
			tokenA: {Subject: subA, Client: "kalke-cli", Email: "a@ex.com", Kind: auth.KindJWT, Scopes: []string{auth.ScopeExtractWrite}},
			tokenB: {Subject: subB, Client: "kalke-cli", Email: "b@ex.com", Kind: auth.KindJWT, Scopes: []string{auth.ScopeExtractWrite}},
		}},
	})

	// Owner can read.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/extractions/"+ownerID, nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner get status %d body %s", rec.Code, rec.Body.String())
	}

	// Other user cannot read by id.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/extractions/"+ownerID, nil)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("idor get status %d body %s", rec.Code, rec.Body.String())
	}

	// Other user list does not include owner row.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/extractions?doc_type=curriculum_vitae", nil)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status %d", rec.Code)
	}
	var body struct {
		Extractions []store.ExtractionSummary `json:"extractions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Extractions) != 0 {
		t.Fatalf("expected empty list for other user, got %#v", body.Extractions)
	}
}

func TestM2MUserForwardRequiresSharedSecret(t *testing.T) {
	const (
		m2mToken = "m2m-token"
		fwdSec   = "forward-secret"
		victim   = "victim-sub"
	)
	st := &memoryStore{}
	_ = st.Insert(context.Background(), store.ExtractionRecord{
		DocType:       "curriculum_vitae",
		ContentSHA256: "xyz",
		Filename:      "v.pdf",
		AuthSubject:   victim,
		Status:        "success",
		Result: extract.Result{
			DocType: "curriculum_vitae",
			Data:    map[string]any{"full_name": "Victim"},
		},
	})

	h := mustHandler(t, httpapi.Deps{
		Extractor:            &stubExtractor{},
		Extractions:          st,
		M2MUserForwardSecret: fwdSec,
		Auth: &stubAuth{principals: map[string]auth.Principal{
			m2mToken: {
				Subject: "service-account-pde-m2m",
				Client:  "pde-m2m",
				Email:   "",
				Kind:    auth.KindJWT,
				Scopes:  []string{auth.ScopeExtractWrite},
			},
		}},
	})

	listAs := func(withSecret bool) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/extractions?doc_type=curriculum_vitae", nil)
		req.Header.Set("Authorization", "Bearer "+m2mToken)
		req.Header.Set("X-Kalke-User-Sub", victim)
		req.Header.Set("X-Kalke-User-Email", "v@ex.com")
		if withSecret {
			req.Header.Set("X-Kalke-Forward-Secret", fwdSec)
		} else {
			req.Header.Set("X-Kalke-Forward-Secret", "wrong")
		}
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("list status %d secret=%v body %s", rec.Code, withSecret, rec.Body.String())
		}
		var body struct {
			Extractions []store.ExtractionSummary `json:"extractions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return len(body.Extractions)
	}

	if n := listAs(false); n != 0 {
		t.Fatalf("forged headers without valid secret must not see victim CVs; got %d", n)
	}
	if n := listAs(true); n != 1 {
		t.Fatalf("BFF with shared secret should see victim CVs; got %d", n)
	}
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
