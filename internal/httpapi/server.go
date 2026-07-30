package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/kalke/personal-document-extractor/internal/extract"
	"github.com/kalke/personal-document-extractor/internal/preprocess"
	"github.com/kalke/personal-document-extractor/internal/store"
)

const (
	persistTimeout    = 3 * time.Second
	readyPingTimeout  = 2 * time.Second
	multipartOverhead = 1 << 20
)

type Server struct {
	extractor   Extractor
	pool        DBPinger
	cache       ResultCache
	extractions ExtractionStore
}

func New(deps Deps) (http.Handler, error) {
	if deps.Extractor == nil {
		return nil, fmt.Errorf("extractor is required")
	}
	s := &Server{
		extractor:   deps.Extractor,
		pool:        deps.Pool,
		cache:       deps.Cache,
		extractions: deps.Extractions,
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(accessLog)

	r.Get("/health", s.health)
	r.Get("/ready", s.ready)
	r.Post("/v1/extract", s.extract)
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
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"doc_types": s.extractor.KnownTypes(),
	})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	dbOK := false
	if s.pool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), readyPingTimeout)
		dbOK = s.pool.Ping(ctx) == nil
		cancel()
	}
	redisOK := false
	if s.cache != nil {
		ctx, cancel := context.WithTimeout(r.Context(), readyPingTimeout)
		redisOK = s.cache.Ping(ctx) == nil
		cancel()
	}

	status := http.StatusOK
	body := map[string]any{
		"status": "ready",
		"db":     dbOK,
		"redis":  redisOK,
	}
	if !dbOK {
		status = http.StatusServiceUnavailable
		body["status"] = "not_ready"
	}
	writeJSON(w, status, body)
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

	doc, err := preprocess.Prepare(filename, data)
	if err != nil {
		log.Warn("preprocess failed", "err", err, "doc_type", docType)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	sum := sha256.Sum256(data)
	shaHex := hex.EncodeToString(sum[:])
	log = log.With("doc_type", docType, "content_sha256", shaHex, "refresh", refresh)

	if !refresh {
		if cached, ok := s.lookupCache(r.Context(), docType, shaHex); ok {
			log.Info("extract", "cache", "hit")
			writeJSON(w, http.StatusOK, toExtractResponse(cached))
			return
		}
	} else {
		s.invalidateCache(r.Context(), docType, shaHex)
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
		reqID:    reqID,
		docType:  docType,
		shaHex:   shaHex,
		filename: filename,
		doc:      doc,
		result:   result,
		duration: time.Since(start),
		refresh:  refresh,
		log:      log,
	})
	log.Info("extract", "cache", "miss", "mode", result.Meta.Mode, "images", result.Meta.Images)
	writeJSON(w, http.StatusOK, toExtractResponse(result))
}

type persistArgs struct {
	reqID    string
	docType  string
	shaHex   string
	filename string
	doc      preprocess.PreparedDocument
	result   extract.Result
	duration time.Duration
	refresh  bool
	log      *slog.Logger
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

	if s.cache != nil {
		cached := args.result
		cached.Meta.Cache = ""
		s.cache.Set(ctx, args.docType, args.shaHex, cached)
	}
	if s.extractions == nil {
		return
	}

	rec := store.ExtractionRecord{
		DocType:       args.docType,
		ContentSHA256: args.shaHex,
		Filename:      args.filename,
		MIME:          args.doc.MIME,
		Mode:          args.result.Meta.Mode,
		Model:         args.result.Meta.Model,
		RequestID:     args.reqID,
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
		args.log.Error("persist extraction failed", "err", err, "refresh", args.refresh)
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

func mapExtractError(err error) (int, string) {
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
