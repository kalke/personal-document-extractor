package httpapi

import (
	"net"
	"net/http"
	"strings"
)

const maxUserAgentLen = 512

func (s *Server) requestOrigin(r *http.Request) (ip, userAgent string) {
	// Auth BFF proxies with Go's net/http; honor browser origin only when the
	// shared forward secret is present (same trust as X-Kalke-User-*).
	if s.acceptsUserForward(r) {
		if fwd := normalizeIP(r.Header.Get(headerClientIP)); fwd != "" {
			ip = fwd
		}
		if ua := strings.TrimSpace(r.Header.Get(headerClientUA)); ua != "" {
			userAgent = truncate(ua, maxUserAgentLen)
		}
	}
	if ip == "" {
		ip = clientIP(r, s.trustedProxies)
	}
	if userAgent == "" {
		userAgent = truncate(strings.TrimSpace(r.UserAgent()), maxUserAgentLen)
	}
	return ip, userAgent
}

// clientIP returns RemoteAddr by default. X-Real-IP is used only when the
// immediate peer is in trustedProxies (Caddy). Client-supplied X-Forwarded-For
// is ignored.
func clientIP(r *http.Request, trusted []*net.IPNet) string {
	remote := remoteAddrIP(r)
	if remote != "" && ipInNets(remote, trusted) {
		if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
			if ip := normalizeIP(xri); ip != "" {
				return ip
			}
		}
	}
	return remote
}

func remoteAddrIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return normalizeIP(host)
	}
	return normalizeIP(r.RemoteAddr)
}

func ipInNets(ipStr string, nets []*net.IPNet) bool {
	if len(nets) == 0 {
		return false
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

func normalizeIP(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	if ip := net.ParseIP(s); ip != nil {
		return ip.String()
	}
	return ""
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}
