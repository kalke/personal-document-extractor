package invoice_nf_test

import (
	"testing"

	"github.com/kalke/personal-document-extractor/internal/doctypes/invoice_nf"
)

func TestNormalize(t *testing.T) {
	r := &invoice_nf.Result{
		Numero: " 1 ",
		Emitente: invoice_nf.Party{
			Nome: " Acme ",
			CNPJ: "04.252.011/0001-10",
		},
		Itens: nil,
	}
	invoice_nf.DocType{}.Normalize(r)
	if r.Numero != "1" {
		t.Fatalf("numero: %q", r.Numero)
	}
	if r.Emitente.CNPJ != "04252011000110" {
		t.Fatalf("cnpj: %q", r.Emitente.CNPJ)
	}
	if r.Itens == nil {
		t.Fatal("itens should be non-nil empty slice")
	}
}
