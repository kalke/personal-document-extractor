package httpapi

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	trusted10 := []*net.IPNet{mustCIDR(t, "10.0.0.0/8")}

	cases := []struct {
		name    string
		remote  string
		xff     string
		xri     string
		trusted []*net.IPNet
		want    string
	}{
		{"remote", "203.0.113.10:54321", "", "", nil, "203.0.113.10"},
		{"xff_ignored_untrusted", "10.0.0.1:1234", "198.51.100.7, 10.0.0.1", "", nil, "10.0.0.1"},
		{"xri_ignored_untrusted", "10.0.0.1:1234", "", "198.51.100.8", nil, "10.0.0.1"},
		{"xff_trusted", "10.0.0.1:1234", "198.51.100.7, 10.0.0.1", "", trusted10, "198.51.100.7"},
		{"xri_trusted", "10.0.0.1:1234", "", "198.51.100.8", trusted10, "198.51.100.8"},
		{"xff_over_xri_trusted", "10.0.0.1:1234", "198.51.100.9", "198.51.100.8", trusted10, "198.51.100.9"},
		{"xff_peer_not_in_list", "203.0.113.10:9", "198.51.100.7", "", trusted10, "203.0.113.10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remote
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if tc.xri != "" {
				r.Header.Set("X-Real-IP", tc.xri)
			}
			if got := clientIP(r, tc.trusted); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestRequestOriginTruncatesUserAgent(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:9"
	r.Header.Set("User-Agent", string(make([]byte, maxUserAgentLen+40)))
	_, ua := s.requestOrigin(r)
	if len(ua) != maxUserAgentLen {
		t.Fatalf("len=%d", len(ua))
	}
}

func mustCIDR(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
