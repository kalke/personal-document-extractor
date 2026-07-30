package address_proof_test

import (
	"testing"

	"github.com/kalke/personal-document-extractor/internal/doctypes/address_proof"
)

func TestNormalize(t *testing.T) {
	data := "01/02/2024"
	r := &address_proof.Result{
		Nome:       " Fulano ",
		Logradouro: " Rua A ",
		UF:         "sp",
		CEP:        "01310-100",
		Data:       &data,
	}
	address_proof.DocType{}.Normalize(r)
	if r.UF != "SP" || r.CEP != "01310100" {
		t.Fatalf("got uf=%q cep=%q", r.UF, r.CEP)
	}
	r.UF = "São Paulo"
	address_proof.DocType{}.Normalize(r)
	if r.UF != "" {
		t.Fatalf("invalid uf should clear, got %q", r.UF)
	}
	if r.Data == nil || *r.Data != "2024-02-01" {
		t.Fatalf("data: %v", r.Data)
	}
}
