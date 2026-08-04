package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/kalke/personal-document-extractor/internal/auth"
	"github.com/kalke/personal-document-extractor/internal/extract"
	"github.com/kalke/personal-document-extractor/internal/preprocess"
	"github.com/kalke/personal-document-extractor/internal/store"
)

const (
	persistTimeout    = 3 * time.Second
	readyPingTimeout  = 2 * time.Second
	multipartOverhead = 1 << 20
	headerUserSub     = "X-Kalke-User-Sub"
	headerUserEmail   = "X-Kalke-User-Email"
	headerForwardSec  = "X-Kalke-Forward-Secret"
)

type Server struct {
	extractor            Extractor
	pool                 DBPinger
	cache                ResultCache
	extractions          ExtractionStore
	consents             ConsentStore
	trustedProxies       []*net.IPNet
	corsOrigins          []string
	auth                 Authenticator
	limiter              RateLimiter
	m2mUserForwardSecret string
}

func New(deps Deps) (http.Handler, error) {
	if deps.Extractor == nil {
		return nil, fmt.Errorf("extractor is required")
	}
	if deps.Auth == nil {
		return nil, fmt.Errorf("authenticator is required")
	}
	if deps.RateLimit == nil {
		return nil, fmt.Errorf("rate limiter is required")
	}
	s := &Server{
		extractor:            deps.Extractor,
		pool:                 deps.Pool,
		cache:                deps.Cache,
		extractions:          deps.Extractions,
		consents:             deps.Consents,
		trustedProxies:       deps.TrustedProxies,
		corsOrigins:          deps.CORSOrigins,
		auth:                 deps.Auth,
		limiter:              deps.RateLimit,
		m2mUserForwardSecret: strings.TrimSpace(deps.M2MUserForwardSecret),
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(s.cors)
	r.Use(accessLog)

	r.Get("/health", s.health)
	r.Get("/ready", s.ready)
	r.Route("/v1", func(r chi.Router) {
		r.Use(s.authenticate)
		r.Post("/extract", s.extract)
		r.Get("/extractions", s.listExtractions)
		r.Get("/extractions/{id}", s.getExtraction)
	})
	return r, nil
}

func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		if r.URL.Path == "/health" {
			return
		}
		slog.Info("http",
			"req_id", middleware.GetReqID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	// Opaque liveness only — no capability enumeration on the public surface.
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	dbOK := false
	if s.pool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), readyPingTimeout)
		dbOK = s.pool.Ping(ctx) == nil
		cancel()
	}
	if !dbOK {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

func (s *Server) extract(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	reqID := middleware.GetReqID(r.Context())
	log := slog.With("req_id", reqID)

	docType := strings.TrimSpace(r.URL.Query().Get("doc_type"))
	if docType == "" {
		writeErr(w, http.StatusBadRequest, "missing doc_type query param")
		return
	}
	refresh := truthyQuery(r, "refresh")

	filename, data, err := readUpload(w, r)
	if err != nil {
		writeUploadError(w, err)
		return
	}
	if !consentAccepted(r, docType) {
		policy := store.RequiredConsentPolicy(docType)
		writeErr(w, http.StatusBadRequest, "lgpd consent required (consent="+policy+")")
		return
	}

	// Cheap validation (MIME/extension) before cache — do NOT rasterize PDFs yet.
	_, detectedMIME, err := preprocess.ValidateUpload(filename, data)
	if err != nil {
		status, msg := mapPreprocessError(err)
		log.Warn("upload validation failed", "err", err, "doc_type", docType, "status", status)
		writeErr(w, status, msg)
		return
	}
	sum := sha256.Sum256(data)
	shaHex := hex.EncodeToString(sum[:])
	clientIP, userAgent := s.requestOrigin(r)
	log = log.With(
		"doc_type", docType,
		"content_sha256", shaHex,
		"refresh", refresh,
		"client_ip", clientIP,
	)

	authSubject, authClient, authEmail := s.resolveActor(r)
	s.recordConsent(r.Context(), store.ConsentRecord{
		UserSub:       authSubject,
		UserEmail:     authEmail,
		IP:            clientIP,
		UserAgent:     userAgent,
		PolicyVersion: store.RequiredConsentPolicy(docType),
		ContentSHA256: shaHex,
		DocType:       docType,
		AcceptedAt:    time.Now().UTC(),
	}, log)

	if !refresh {
		if cached, ok := s.lookupCache(r.Context(), docType, shaHex); ok {
			cached.Meta.ContentSHA256 = shaHex
			cached.Meta.Cache = "hit"
			// Still attribute a stored copy to this user (esp. CV history) without burning LLM quota.
			s.persist(persistArgs{
				reqID:       reqID,
				docType:     docType,
				shaHex:      shaHex,
				filename:    filename,
				doc:         preprocess.PreparedDocument{MIME: detectedMIME, Filename: filename},
				result:      cached,
				duration:    time.Since(start),
				refresh:     false,
				clientIP:    clientIP,
				userAgent:   userAgent,
				authSubject: authSubject,
				authClient:  authClient,
				authEmail:   authEmail,
				log:         log,
			})
			log.Info("extract", "cache", "hit")
			writeJSON(w, http.StatusOK, toExtractResponse(cached))
			return
		}
	} else {
		s.invalidateCache(r.Context(), docType, shaHex)
	}

	// Rate limit only LLM/cache-miss work — cached re-reads are free.
	if !s.enforceLLMRateLimit(w, r) {
		return
	}

	doc, err := preprocess.Prepare(filename, data)
	if err != nil {
		status, msg := mapPreprocessError(err)
		log.Warn("preprocess failed", "err", err, "doc_type", docType, "status", status)
		writeErr(w, status, msg)
		return
	}

	result, err := s.extractor.Extract(r.Context(), docType, doc)
	if err != nil {
		status, msg := mapExtractError(err)
		log.Error("extract failed", "err", err, "cache", "miss", "status", status)
		writeErr(w, status, msg)
		return
	}

	result.Meta.ContentSHA256 = shaHex
	result.Meta.Cache = "miss"
	s.persist(persistArgs{
		reqID:       reqID,
		docType:     docType,
		shaHex:      shaHex,
		filename:    filename,
		doc:         doc,
		result:      result,
		duration:    time.Since(start),
		refresh:     refresh,
		clientIP:    clientIP,
		userAgent:   userAgent,
		authSubject: authSubject,
		authClient:  authClient,
		authEmail:   authEmail,
		log:         log,
	})
	log.Info("extract", "cache", "miss", "mode", result.Meta.Mode, "images", result.Meta.Images)
	writeJSON(w, http.StatusOK, toExtractResponse(result))
}

type persistArgs struct {
	reqID       string
	docType     string
	shaHex      string
	filename    string
	doc         preprocess.PreparedDocument
	result      extract.Result
	duration    time.Duration
	refresh     bool
	clientIP    string
	userAgent   string
	authSubject string
	authClient  string
	authEmail   string
	log         *slog.Logger
}

func (s *Server) lookupCache(ctx context.Context, docType, shaHex string) (extract.Result, bool) {
	if s.cache == nil {
		return extract.Result{}, false
	}
	return s.cache.Get(ctx, docType, shaHex)
}

func (s *Server) invalidateCache(ctx context.Context, docType, shaHex string) {
	if s.cache == nil {
		return
	}
	s.cache.Delete(ctx, docType, shaHex)
}

func (s *Server) persist(args persistArgs) {
	ctx, cancel := context.WithTimeout(context.Background(), persistTimeout)
	defer cancel()

	if s.extractions != nil {
		rec := store.ExtractionRecord{
			DocType:       args.docType,
			ContentSHA256: args.shaHex,
			Filename:      args.filename,
			MIME:          args.doc.MIME,
			Mode:          args.result.Meta.Mode,
			Model:         args.result.Meta.Model,
			RequestID:     args.reqID,
			ClientIP:      args.clientIP,
			UserAgent:     args.userAgent,
			AuthSubject:   args.authSubject,
			AuthClient:    args.authClient,
			AuthEmail:     args.authEmail,
			Status:        "success",
			Result:        args.result,
			Duration:      args.duration,
		}

		var err error
		if args.refresh {
			err = s.extractions.Replace(ctx, rec)
		} else {
			err = s.extractions.Insert(ctx, rec)
		}
		if err != nil {
			// Do not populate Redis on DB failure — otherwise cache hits skip persist forever.
			args.log.Error("persist extraction failed", "err", err, "refresh", args.refresh)
			return
		}
	}

	if s.cache != nil {
		cached := args.result
		cached.Meta.Cache = ""
		s.cache.Set(ctx, args.docType, args.shaHex, cached)
	}
}

func truthyQuery(r *http.Request, key string) bool {
	v := strings.TrimSpace(strings.ToLower(r.URL.Query().Get(key)))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func consentAccepted(r *http.Request, docType string) bool {
	v := strings.TrimSpace(r.FormValue("consent"))
	if v == "" {
		v = strings.TrimSpace(r.Header.Get("X-LGPD-Consent"))
	}
	required := store.RequiredConsentPolicy(docType)
	if strings.EqualFold(v, required) {
		return true
	}
	// Legacy boolean-ish values only for the original extract policy.
	if required == store.PolicyLGPDExtractV1 {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "y", "on":
			return true
		}
	}
	return false
}

// resolveActor attributes persistence/consent to the end user when the Auth BFF
// calls with an M2M token and forwards X-Kalke-User-* with a shared secret.
// Without a valid forward secret, user headers are ignored (fail closed).
func (s *Server) resolveActor(r *http.Request) (subject, client, email string) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		return "", "", ""
	}
	subject, client, email = p.Subject, p.Client, p.Email
	if !isM2MPrincipal(p) {
		return subject, client, email
	}
	if !s.acceptsUserForward(r) {
		return subject, client, email
	}
	if sub := strings.TrimSpace(r.Header.Get(headerUserSub)); sub != "" {
		subject = sub
		if em := strings.TrimSpace(r.Header.Get(headerUserEmail)); em != "" {
			email = em
		}
	}
	return subject, client, email
}

func (s *Server) acceptsUserForward(r *http.Request) bool {
	if s == nil || s.m2mUserForwardSecret == "" {
		return false
	}
	got := strings.TrimSpace(r.Header.Get(headerForwardSec))
	if got == "" {
		return false
	}
	want := s.m2mUserForwardSecret
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func isM2MPrincipal(p auth.Principal) bool {
	if p.Kind != auth.KindJWT {
		return false
	}
	if p.Client == "pde-m2m" {
		return true
	}
	// Service-account tokens usually have no email.
	return p.Email == "" && strings.HasPrefix(p.Client, "pde-")
}

func (s *Server) listExtractions(w http.ResponseWriter, r *http.Request) {
	if s.extractions == nil {
		writeErr(w, http.StatusServiceUnavailable, "extractions unavailable")
		return
	}
	subject, _, _ := s.resolveActor(r)
	if subject == "" {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	docType := strings.TrimSpace(r.URL.Query().Get("doc_type"))
	limit := 20
	items, err := s.extractions.ListBySubject(r.Context(), subject, docType, limit)
	if err != nil {
		slog.Error("list extractions", "err", err, "subject", subject)
		writeErr(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"extractions": items})
}

func (s *Server) getExtraction(w http.ResponseWriter, r *http.Request) {
	if s.extractions == nil {
		writeErr(w, http.StatusServiceUnavailable, "extractions unavailable")
		return
	}
	subject, _, _ := s.resolveActor(r)
	if subject == "" {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	item, err := s.extractions.GetByIDForSubject(r.Context(), id, subject)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		slog.Error("get extraction", "err", err, "id", id)
		writeErr(w, http.StatusInternalServerError, "get failed")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) recordConsent(ctx context.Context, rec store.ConsentRecord, log *slog.Logger) {
	if s.consents == nil {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, persistTimeout)
	defer cancel()
	if err := s.consents.Insert(cctx, rec); err != nil {
		log.Error("persist extract consent failed", "err", err)
	}
}

func readUpload(w http.ResponseWriter, r *http.Request) (filename string, data []byte, err error) {
	r.Body = http.MaxBytesReader(w, r.Body, preprocess.MaxUploadBytes+multipartOverhead)
	if err := r.ParseMultipartForm(preprocess.MaxUploadBytes); err != nil {
		return "", nil, err
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return "", nil, errMissingFile
	}
	defer func() { _ = file.Close() }()

	data, err = io.ReadAll(io.LimitReader(file, preprocess.MaxUploadBytes+1))
	if err != nil {
		return "", nil, err
	}
	if len(data) == 0 {
		return "", nil, preprocess.ErrEmptyUpload
	}
	if len(data) > preprocess.MaxUploadBytes {
		return "", nil, errTooLarge
	}
	return header.Filename, data, nil
}

var (
	errMissingFile = errors.New("multipart field 'file' is required")
	errTooLarge    = errors.New("uploaded file exceeds size limit")
)

func writeUploadError(w http.ResponseWriter, err error) {
	var maxBytes *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytes), errors.Is(err, errTooLarge):
		writeErr(w, http.StatusRequestEntityTooLarge, "uploaded file exceeds size limit")
	case errors.Is(err, errMissingFile):
		writeErr(w, http.StatusBadRequest, errMissingFile.Error())
	case errors.Is(err, preprocess.ErrEmptyUpload):
		writeErr(w, http.StatusBadRequest, preprocess.ErrEmptyUpload.Error())
	default:
		writeErr(w, http.StatusBadRequest, "invalid multipart form")
	}
}

func mapPreprocessError(err error) (int, string) {
	switch {
	case errors.Is(err, preprocess.ErrEmptyUpload):
		return http.StatusBadRequest, preprocess.ErrEmptyUpload.Error()
	case errors.Is(err, preprocess.ErrUnsupportedMedia):
		return http.StatusBadRequest, preprocess.ErrUnsupportedMedia.Error()
	case errors.Is(err, preprocess.ErrMIMEMismatch):
		return http.StatusBadRequest, preprocess.ErrMIMEMismatch.Error()
	case errors.Is(err, preprocess.ErrUploadTooLarge):
		return http.StatusRequestEntityTooLarge, preprocess.ErrUploadTooLarge.Error()
	default:
		return http.StatusBadRequest, "invalid document"
	}
}

func mapExtractError(err error) (int, string) {
	msg := err.Error()
	if strings.Contains(msg, "groq http 429") || strings.Contains(msg, "Rate limit reached") {
		return http.StatusTooManyRequests, "extraction provider busy; try again in a minute"
	}
	if strings.Contains(msg, "groq http 413") || strings.Contains(msg, "Request too large") {
		return http.StatusRequestEntityTooLarge, "document too large for extraction provider; try a shorter CV or fewer pages"
	}
	switch {
	case errors.Is(err, extract.ErrUnknownDocType):
		return http.StatusBadRequest, "unknown doc_type"
	case errors.Is(err, extract.ErrInvalidJSON):
		return http.StatusUnprocessableEntity, "could not process document"
	case errors.Is(err, extract.ErrLLM):
		return http.StatusBadGateway, "extraction provider unavailable"
	default:
		return http.StatusInternalServerError, "could not process document"
	}
}

type errorBody struct {
	Error string `json:"error"`
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json failed", "err", err)
	}
}
