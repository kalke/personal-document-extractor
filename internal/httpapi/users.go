package httpapi

import (
	"net/http"

	"github.com/kalke/personal-document-extractor/internal/auth"
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
	p, ok := requireUser(w, r)
	if !ok {
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

func requireUser(w http.ResponseWriter, r *http.Request) (auth.Principal, bool) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.UserID == "" {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return auth.Principal{}, false
	}
	return p, true
}
