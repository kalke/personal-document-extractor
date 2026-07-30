package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kalke/personal-document-extractor/internal/auth"
	"github.com/kalke/personal-document-extractor/internal/authz"
	"github.com/kalke/personal-document-extractor/internal/store"
)

type meResponse struct {
	ID            string   `json:"id"`
	AuthSubject   string   `json:"auth_subject"`
	Email         string   `json:"email,omitempty"`
	EmailVerified bool     `json:"email_verified"`
	DisplayName   string   `json:"display_name,omitempty"`
	Status        string   `json:"status"`
	Kind          string   `json:"auth_kind"`
	Scopes        []string `json:"scopes"`
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.UserID == "" {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if s.users == nil {
		writeErr(w, http.StatusInternalServerError, "users not configured")
		return
	}
	u, err := s.users.GetByID(r.Context(), p.UserID)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if u.Status != "active" {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, meResponse{
		ID:            u.ID,
		AuthSubject:   u.AuthSubject,
		Email:         u.Email,
		EmailVerified: u.EmailVerified,
		DisplayName:   u.DisplayName,
		Status:        u.Status,
		Kind:          string(p.Kind),
		Scopes:        p.Scopes,
	})
}

type apiKeyResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	KeyPrefix string     `json:"key_prefix"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type createAPIKeyRequest struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

type createAPIKeyResponse struct {
	apiKeyResponse
	Secret string `json:"secret"`
}

func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.UserID == "" {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	keys, err := s.apiKeys.ListByUser(r.Context(), p.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list api keys")
		return
	}
	out := make([]apiKeyResponse, 0, len(keys))
	for _, k := range keys {
		out = append(out, toAPIKeyResponse(k))
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": out})
}

func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.UserID == "" {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req createAPIKeyRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "default"
	}
	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = []string{auth.ScopeExtractWrite}
	}
	for _, sc := range scopes {
		if sc == auth.ScopeAdmin && !authz.HasScope(p, auth.ScopeAdmin) {
			writeErr(w, http.StatusForbidden, "cannot create admin keys")
			return
		}
		switch sc {
		case auth.ScopeExtractWrite, auth.ScopeKeysManage, auth.ScopeAdmin:
		default:
			writeErr(w, http.StatusBadRequest, "unknown scope")
			return
		}
	}
	plaintext, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create api key")
		return
	}
	rec, err := s.apiKeys.Create(r.Context(), store.CreateAPIKeyInput{
		UserID:    p.UserID,
		Name:      name,
		KeyPrefix: prefix,
		KeyHash:   hash,
		Scopes:    scopes,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create api key")
		return
	}
	writeJSON(w, http.StatusCreated, createAPIKeyResponse{
		apiKeyResponse: apiKeyResponse{
			ID:        rec.ID,
			Name:      rec.Name,
			KeyPrefix: rec.KeyPrefix,
			Scopes:    rec.Scopes,
			ExpiresAt: rec.ExpiresAt,
			CreatedAt: rec.CreatedAt,
		},
		Secret: plaintext,
	})
}

func (s *Server) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.UserID == "" {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeErr(w, http.StatusBadRequest, "missing key id")
		return
	}
	if err := s.apiKeys.RevokeForUser(r.Context(), p.UserID, id); err != nil {
		if errors.Is(err, store.ErrAPIKeyNotFound) {
			writeErr(w, http.StatusNotFound, "api key not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "could not revoke api key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toAPIKeyResponse(k store.APIKeyPublic) apiKeyResponse {
	return apiKeyResponse{
		ID:        k.ID,
		Name:      k.Name,
		KeyPrefix: k.KeyPrefix,
		Scopes:    k.Scopes,
		ExpiresAt: k.ExpiresAt,
		RevokedAt: k.RevokedAt,
		CreatedAt: k.CreatedAt,
	}
}
