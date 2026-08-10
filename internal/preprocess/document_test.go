package preprocess

import "testing"

func TestUsablePDFText(t *testing.T) {
	t.Parallel()

	junk := stringsRepeat("Form field /Obj 12 0 R xref trailer ", 20)
	if usablePDFText(junk) {
		t.Fatalf("junk text layer should not count as usable (len=%d)", len(junk))
	}

	short := "nome cpf 123"
	if usablePDFText(short) {
		t.Fatal("short text should not count as usable")
	}

	idLike := "Documento de identidade\nNome: Maria Silva\nCPF: 123.456.789-09\n" +
		"Data de nascimento: 01/02/1990\nNumero do documento: 1234567890\n" +
		"Orgao emissor: SSP/SP\nValidade: 01/01/2030\n" +
		stringsRepeat("texto auxiliar do documento digital ", 4)
	if !usablePDFText(idLike) {
		t.Fatal("identity-like text should be usable")
	}

	longCV := stringsRepeat("Experiencia profissional em engenharia de software com Go e Python. ", 20)
	if !usablePDFText(longCV) {
		t.Fatal("long CV text should be usable")
	}
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
