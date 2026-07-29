package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

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
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", s.health)
	r.Post("/v1/extract", s.extract)
	return r
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"doc_types": s.extractor.KnownTypes(),
	})
}

func (s *Server) extract(w http.ResponseWriter, r *http.Request) {
	docType := strings.TrimSpace(r.URL.Query().Get("doc_type"))
	if docType == "" {
		writeErr(w, http.StatusBadRequest, "missing doc_type query param")
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
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

	name := strings.ToLower(header.Filename)
	if !strings.HasSuffix(name, ".pdf") && !strings.Contains(header.Header.Get("Content-Type"), "pdf") {
		writeErr(w, http.StatusBadRequest, "only PDF files are supported in MVP")
		return
	}

	text, err := preprocess.TextFromPDF(data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := s.extractor.Extract(r.Context(), docType, text)
	if err != nil {
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
	_ = json.NewEncoder(w).Encode(v)
}
