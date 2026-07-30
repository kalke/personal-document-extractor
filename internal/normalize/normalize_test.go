package normalize

import "testing"

func TestCPF(t *testing.T) {
	if got := CPF("529.982.247-25"); got != "52998224725" {
		t.Fatalf("valid masked: got %q", got)
	}
	if got := CPF("52998224725"); got != "52998224725" {
		t.Fatalf("valid digits: got %q", got)
	}
	if got := CPF("9908198340522"); got != "" {
		t.Fatalf("absurd length should clear: got %q", got)
	}
	if got := CPF("111.111.111-11"); got != "" {
		t.Fatalf("invalid checksum should clear: got %q", got)
	}
	if got := CPF(""); got != "" {
		t.Fatalf("empty: got %q", got)
	}
}

func TestDateToISO(t *testing.T) {
	cases := map[string]*string{
		"15/01/1990":           strPtr("1990-01-15"),
		"31/12/2030":           strPtr("2030-12-31"),
		"1990-01-15":           strPtr("1990-01-15"),
		"15/01/1990 Sao Paulo": strPtr("1990-01-15"),
		"":                     nil,
		"not-a-date":           nil,
	}
	for in, want := range cases {
		got := DateToISO(in)
		if (got == nil) != (want == nil) {
			t.Fatalf("%q: got %v want %v", in, got, want)
		}
		if got != nil && want != nil && *got != *want {
			t.Fatalf("%q: got %q want %q", in, *got, *want)
		}
	}
}

func strPtr(s string) *string { return &s }
