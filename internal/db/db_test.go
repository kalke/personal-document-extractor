package db

import "testing"

func TestNormalizeSearchPath(t *testing.T) {
	got, err := NormalizeSearchPath("")
	if err != nil || got != DefaultSchema {
		t.Fatalf("empty: got %q err=%v", got, err)
	}
	got, err = NormalizeSearchPath("pde")
	if err != nil || got != "pde" {
		t.Fatalf("pde: got %q err=%v", got, err)
	}
	if _, err := NormalizeSearchPath("pde; drop"); err == nil {
		t.Fatal("expected invalid schema error")
	}
	if SearchPathRuntime("pde") != "pde, public" {
		t.Fatalf("runtime=%q", SearchPathRuntime("pde"))
	}
}
