package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/kalke/personal-document-extractor/internal/auth"
)

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := map[string]struct{}{}
	for _, o := range s.corsOrigins {
		o = strings.TrimSpace(o)
		if o != "" {
			allowed[o] = struct{}{}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

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

// enforceLLMRateLimit consumes one quota unit for a Groq/cache-miss extraction.
// Returns false when the response was already written (blocked or unavailable).
func (s *Server) enforceLLMRateLimit(w http.ResponseWriter, r *http.Request) bool {
	if s.limiter == nil {
		writeErr(w, http.StatusTooManyRequests, "rate limit unavailable")
		return false
	}
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	id := p.Subject
	// Attribute playground quota to the end user when BFF forwards identity.
	if isM2MPrincipal(p) {
		if sub := strings.TrimSpace(r.Header.Get(headerUserSub)); sub != "" {
			id = sub
		}
	}
	if id == "" {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	res, err := s.limiter.Allow(r.Context(), string(p.Kind), id)
	if err != nil {
		writeErr(w, http.StatusTooManyRequests, "rate limit unavailable")
		return false
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
		return false
	}
	return true
}
