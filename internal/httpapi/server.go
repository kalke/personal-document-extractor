package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/kalke/personal-document-extractor/internal/extract"
	"github.com/kalke/personal-document-extractor/internal/preprocess"
)

type Server struct {
	extractor *extract.Service
}

func New(extractor *extract.Service) http.Handler {
	s := &Server{extractor: extractor}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(accessLog)

	r.Get("/health", s.health)
	r.Post("/v1/extract", s.extract)
	return r
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

func (s *Server) extract(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	log := slog.With("req_id", middleware.GetReqID(r.Context()))

	docType := strings.TrimSpace(r.URL.Query().Get("doc_type"))
	if docType == "" {
		writeErr(w, http.StatusBadRequest, "missing doc_type query param")
		return
	}

	if err := r.ParseMultipartForm(preprocess.MaxUploadBytes); err != nil {
		log.Warn("multipart parse failed", "err", err)
		writeErr(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "multipart field 'file' is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "failed to read uploaded file")
		return
	}
	if len(data) == 0 {
		writeErr(w, http.StatusBadRequest, "uploaded file is empty")
		return
	}

	log.Info("extract received",
		"doc_type", docType,
		"filename", header.Filename,
		"bytes", len(data),
	)

	doc, err := preprocess.Prepare(header.Filename, header.Header.Get("Content-Type"), data)
	if err != nil {
		log.Warn("preprocess failed", "err", err)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := s.extractor.Extract(r.Context(), docType, doc)
	if err != nil {
		log.Error("extract failed",
			"doc_type", docType,
			"err", err,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		switch {
		case errors.Is(err, extract.ErrUnknownDocType):
			writeErr(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, extract.ErrInvalidJSON):
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, extract.ErrLLM):
			writeErr(w, http.StatusBadGateway, err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	log.Info("extract done",
		"doc_type", result.DocType,
		"mode", result.Meta.Mode,
		"images", result.Meta.Images,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	writeJSON(w, http.StatusOK, result)
}

type errorBody struct {
	Error string `json:"error"`
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json failed", "err", err)
	}
}
