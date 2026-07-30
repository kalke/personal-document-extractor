package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/kalke/personal-document-extractor/internal/auth"
	"github.com/kalke/personal-document-extractor/internal/authz"
)

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			writeErr(w, http.StatusInternalServerError, "authentication not configured")
			return
		}
		raw := strings.TrimSpace(r.Header.Get("Authorization"))
		const prefix = "Bearer "
		if !strings.HasPrefix(raw, prefix) {
			writeErr(w, http.StatusUnauthorized, "missing or invalid authorization")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(raw, prefix))
		principal, err := s.auth.Authenticate(r.Context(), token)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
	})
}

func (s *Server) requireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := auth.PrincipalFromContext(r.Context())
			if !ok || !authz.HasScope(p, scope) {
				writeErr(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.limiter == nil {
			writeErr(w, http.StatusTooManyRequests, "rate limit unavailable")
			return
		}
		p, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		id := p.Subject
		if id == "" {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		res, err := s.limiter.Allow(r.Context(), string(p.Kind), id)
		if err != nil {
			writeErr(w, http.StatusTooManyRequests, "rate limit unavailable")
			return
		}
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
		if !res.Allowed {
			sec := int(res.RetryAfter.Seconds())
			if sec < 1 {
				sec = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(sec))
			writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}
