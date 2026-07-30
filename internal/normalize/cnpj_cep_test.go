package normalize

import "testing"

func TestDigitsOnly(t *testing.T) {
	if got := DigitsOnly("a1b2-3"); got != "123" {
		t.Fatalf("got %q", got)
	}
}

func TestCNPJ(t *testing.T) {
	if got := CNPJ("04.252.011/0001-10"); got != "04252011000110" {
		t.Fatalf("valid masked: got %q", got)
	}
	if got := CNPJ("04252011000110"); got != "04252011000110" {
		t.Fatalf("valid digits: got %q", got)
	}
	if got := CNPJ("11.111.111/1111-11"); got != "" {
		t.Fatalf("invalid should clear: got %q", got)
	}
}

func TestCEP(t *testing.T) {
	if got := CEP("01310-100"); got != "01310100" {
		t.Fatalf("got %q", got)
	}
	if got := CEP("1310100"); got != "" {
		t.Fatalf("wrong length should clear: got %q", got)
	}
}

func TestValidCPFRejectsAllSame(t *testing.T) {
	if ValidCPF("11111111111") {
		t.Fatal("all-same digits must be invalid")
	}
}
