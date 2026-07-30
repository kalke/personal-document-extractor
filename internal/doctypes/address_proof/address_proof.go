package address_proof

import (
	"strings"

	"github.com/kalke/personal-document-extractor/internal/normalize"
)

const Name = "address_proof"

type Result struct {
	Nome       string  `json:"nome"`
	Logradouro string  `json:"logradouro"`
	Numero     string  `json:"numero"`
	Bairro     string  `json:"bairro"`
	Cidade     string  `json:"cidade"`
	UF         string  `json:"uf"`
	CEP        string  `json:"cep"`
	Emissor    string  `json:"emissor"`
	Data       *string `json:"data"`
}

type DocType struct{}

func (DocType) Name() string { return Name }

func (DocType) SystemPrompt() string {
	return `You extract fields from a Brazilian comprovante de endereço (utility bill, bank statement, or residence declaration).
Return one JSON object only. No markdown. No guessing.

Rules:
- nome = account holder / resident name printed on the document
- logradouro / numero / bairro / cidade / uf / cep = address components only
- CEP is exactly 8 digits (#####-### or ########). If unclear, return "".
- data = document issue/reference date only
- Unknown text = "". Unknown dates = null.
- Prefer YYYY-MM-DD for dates (DD/MM/YYYY also ok).`
}

func (DocType) SchemaHint() string {
	return `{
  "nome": "string",
  "logradouro": "string",
  "numero": "string",
  "bairro": "string",
  "cidade": "string",
  "uf": "string — 2-letter UF",
  "cep": "string — exactly 8 digits or empty",
  "emissor": "string",
  "data": "YYYY-MM-DD or null"
}`
}

func (DocType) EmptyResult() any { return &Result{} }

func (DocType) Normalize(result any) {
	r, ok := result.(*Result)
	if !ok || r == nil {
		return
	}
	r.Nome = strings.TrimSpace(r.Nome)
	r.Logradouro = strings.TrimSpace(r.Logradouro)
	r.Numero = strings.TrimSpace(r.Numero)
	r.Bairro = strings.TrimSpace(r.Bairro)
	r.Cidade = strings.TrimSpace(r.Cidade)
	r.UF = strings.ToUpper(strings.TrimSpace(r.UF))
	r.CEP = normalize.CEP(r.CEP)
	r.Emissor = strings.TrimSpace(r.Emissor)
	r.Data = normalize.DateToISO(ptrString(r.Data))
}

func ptrString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
