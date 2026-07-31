package auth

import "testing"

func TestRewriteURLOrigin(t *testing.T) {
	got, err := rewriteURLOrigin(
		"http://localhost:8443/realms/kalke/protocol/openid-connect/certs",
		"http://caddy:8443/realms/kalke",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "http://caddy:8443/realms/kalke/protocol/openid-connect/certs"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
