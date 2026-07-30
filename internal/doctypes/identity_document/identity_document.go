package identity_document

import (
	"log/slog"
	"strings"

	"github.com/kalke/personal-document-extractor/internal/normalize"
)

const Name = "identity_document"

type Result struct {
	Tipo            string  `json:"tipo"`
	Nome            string  `json:"nome"`
	CPF             string  `json:"cpf"`
	NumeroDocumento string  `json:"numero_documento"`
	DataNascimento  *string `json:"data_nascimento"`
	OrgaoEmissor    string  `json:"orgao_emissor"`
	Validade        *string `json:"validade"`
}

type DocType struct{}

func (DocType) Name() string { return Name }

func (DocType) SystemPrompt() string {
	return `You read Brazilian RG / CNH / CNH-e and return one JSON object. No markdown.

Brazilian CNH layout (use the numbered fields on the card):
- 2 NOME E SOBRENOME → nome
- 3 DATA E LOCAL DE NASCIMENTO → data_nascimento = the date only (DD/MM/YYYY). Read EVERY digit of the year carefully (9 vs 0 are easy to misread).
- 4a DATA EMISSÃO → ignore for "validade" (this is issue date, NOT expiration)
- 4b VALIDADE → validade (expiration). Must come from the field labeled VALIDADE / 4b, never from 4a.
- 4c DOC IDENTIDADE / ÓRGÃO / UF → orgao_emissor when present
- 4d 1ª HABILITAÇÃO → ignore for validade and birth date
- 5 (CAT) → ignore
- 6 CPF → cpf. Printed as ###.###.###-##. Copy that exact value. Exactly 11 digits after removing punctuation.
- 7 Nº REGISTRO → numero_documento (this is NOT the CPF)
- 8 RENACH / other codes → ignore for cpf and numero_documento

CPF:
- Only the value under/beside label "CPF" (field 6 on CNH).
- Keep punctuation in your reading if helpful, e.g. "123.456.789-09", or digits only — both ok.
- Do not invent digits. Do not glue CPF with registro, dates, or phone-like numbers.
- If the CPF digits are readable, ALWAYS fill cpf. Empty only if the CPF field is truly unreadable.

tipo: "CNH" for carteira de habilitação, "RG" for identity card, else "outro".

For RG: numero_documento = RG number; cpf only from CPF label if present.

Dates: YYYY-MM-DD preferred (DD/MM/YYYY accepted). Unknown text "". Unknown dates null.`
}

func (DocType) SchemaHint() string {
	return `{
  "tipo": "RG | CNH | outro",
  "nome": "string",
  "cpf": "string — CPF from label CPF only (###.###.###-## or 11 digits)",
  "numero_documento": "string — CNH field 7 Nº REGISTRO, or RG number (never CPF)",
  "data_nascimento": "YYYY-MM-DD — from field 3 only",
  "orgao_emissor": "string",
  "validade": "YYYY-MM-DD — from field 4b VALIDADE only, never 4a emissão"
}`
}

func (DocType) EmptyResult() any { return &Result{} }

func (DocType) Normalize(result any) {
	r, ok := result.(*Result)
	if !ok || r == nil {
		return
	}
	r.Tipo = strings.TrimSpace(r.Tipo)
	r.Nome = strings.TrimSpace(r.Nome)
	rawCPF := strings.TrimSpace(r.CPF)
	r.CPF = normalize.CPF(rawCPF)
	if rawCPF != "" && r.CPF == "" {
		slog.Warn("cpf rejected by validation", "raw_len", len(normalize.DigitsOnly(rawCPF)))
	}
	r.NumeroDocumento = strings.TrimSpace(r.NumeroDocumento)
	r.OrgaoEmissor = strings.TrimSpace(r.OrgaoEmissor)
	r.DataNascimento = normalize.DateToISO(ptrString(r.DataNascimento))
	r.Validade = normalize.DateToISO(ptrString(r.Validade))
}

func ptrString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
