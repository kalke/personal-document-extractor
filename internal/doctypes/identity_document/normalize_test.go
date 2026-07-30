package identity_document_test

import (
	"testing"

	"github.com/kalke/personal-document-extractor/internal/doctypes/identity_document"
)

func TestNormalize(t *testing.T) {
	birth := "15/01/1990"
	valid := "31/12/2030"
	r := &identity_document.Result{
		Tipo:            " CNH ",
		Nome:            " FULANO DA SILVA ",
		CPF:             "529.982.247-25",
		NumeroDocumento: " 12.345.678-9 ",
		DataNascimento:  &birth,
		OrgaoEmissor:    " DETRAN/SP ",
		Validade:        &valid,
	}
	identity_document.DocType{}.Normalize(r)

	if r.Tipo != "CNH" || r.Nome != "FULANO DA SILVA" {
		t.Fatalf("normalize failed: %+v", r)
	}
	if r.CPF != "52998224725" {
		t.Fatalf("cpf: %q", r.CPF)
	}
	if r.NumeroDocumento != "123456789" {
		t.Fatalf("numero_documento: %q", r.NumeroDocumento)
	}
	if r.DataNascimento == nil || *r.DataNascimento != "1990-01-15" {
		t.Fatalf("birth: %v", r.DataNascimento)
	}
	if r.Validade == nil || *r.Validade != "2030-12-31" {
		t.Fatalf("validade: %v", r.Validade)
	}
}

func TestNormalizeRejectsBadCPF(t *testing.T) {
	r := &identity_document.Result{CPF: "111.111.111-11"}
	identity_document.DocType{}.Normalize(r)
	if r.CPF != "" {
		t.Fatalf("expected empty cpf, got %q", r.CPF)
	}
}

func TestNormalizeTipo(t *testing.T) {
	r := &identity_document.Result{Tipo: "cnh-e"}
	identity_document.DocType{}.Normalize(r)
	if r.Tipo != "CNH" {
		t.Fatalf("got %q", r.Tipo)
	}
	r.Tipo = "passport"
	identity_document.DocType{}.Normalize(r)
	if r.Tipo != "outro" {
		t.Fatalf("got %q", r.Tipo)
	}
}
