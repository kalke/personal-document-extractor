package httpapi

import (
	"net"
	"net/http"
	"strings"
)

const maxUserAgentLen = 512

func (s *Server) requestOrigin(r *http.Request) (ip, userAgent string) {
	return clientIP(r, s.trustedProxies), truncate(strings.TrimSpace(r.UserAgent()), maxUserAgentLen)
}

// clientIP returns RemoteAddr by default. X-Forwarded-For / X-Real-IP are used
// only when the immediate peer is in trustedProxies (CIDR allowlist).
func clientIP(r *http.Request, trusted []*net.IPNet) string {
	remote := remoteAddrIP(r)
	if remote != "" && ipInNets(remote, trusted) {
		if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
			if ip := firstIP(xff); ip != "" {
				return ip
			}
		}
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

func firstIP(forwardedFor string) string {
	parts := strings.Split(forwardedFor, ",")
	if len(parts) == 0 {
		return ""
	}
	return normalizeIP(parts[0])
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
