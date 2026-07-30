package extract

import (
	"testing"

	"github.com/kalke/personal-document-extractor/internal/doctypes/identity_document"
)

func TestStripFences(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                         `{"a":1}`,
		"```json\n{\"a\":1}\n```":         `{"a":1}`,
		"<think>noise</think>\n{\"a\":1}": `{"a":1}`,
		"```JSON\n{\"ok\":true}\n```":     `{"ok":true}`,
	}
	for in, want := range cases {
		if got := stripFences(in); got != want {
			t.Fatalf("stripFences(%q)=%q want %q", in, got, want)
		}
	}
}

func TestDecodeInto(t *testing.T) {
	dt := identity_document.DocType{}
	raw := "```json\n{\"tipo\":\"CNH\",\"nome\":\"FULANO DA SILVA\",\"cpf\":\"52998224725\",\"numero_documento\":\"123\",\"data_nascimento\":\"1990-01-15\",\"orgao_emissor\":\"DETRAN/SP\",\"validade\":\"2030-12-31\"}\n```"
	got, err := decodeInto(dt, raw)
	if err != nil {
		t.Fatalf("decodeInto: %v", err)
	}
	r, ok := got.(*identity_document.Result)
	if !ok || r.Nome != "FULANO DA SILVA" || r.Tipo != "CNH" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestServiceKnownTypes(t *testing.T) {
	svc := NewService(nil, identity_document.DocType{})
	types := svc.KnownTypes()
	if len(types) != 1 || types[0] != identity_document.Name {
		t.Fatalf("got %v", types)
	}
}
