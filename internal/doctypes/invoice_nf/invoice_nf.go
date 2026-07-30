package invoice_nf

import (
	"strings"

	"github.com/kalke/personal-document-extractor/internal/normalize"
)

const Name = "invoice_nf"

type Party struct {
	Nome string `json:"nome"`
	CNPJ string `json:"cnpj"`
	CPF  string `json:"cpf"`
}

type Item struct {
	Descricao  string  `json:"descricao"`
	Quantidade float64 `json:"quantidade"`
	Valor      float64 `json:"valor"`
}

type Result struct {
	Numero       string  `json:"numero"`
	Serie        string  `json:"serie"`
	Emitente     Party   `json:"emitente"`
	Destinatario Party   `json:"destinatario"`
	ValorTotal   float64 `json:"valor_total"`
	DataEmissao  *string `json:"data_emissao"`
	Itens        []Item  `json:"itens"`
}

type DocType struct{}

func (DocType) Name() string { return Name }

func (DocType) SystemPrompt() string {
	return `You extract fields from a Brazilian nota fiscal (NFe or NFSe).
Return one JSON object only. No markdown. No guessing.

Identifier rules:
- CPF: label "CPF", pattern ###.###.###-##, exactly 11 digits. Never invent.
- CNPJ: label "CNPJ", pattern ##.###.###/####-##, exactly 14 digits. Never invent.
- Do not swap emitente and destinatario.
- Amounts are JSON numbers. Unknown text "". Unknown numbers 0. No items []. Unknown dates null.
- Dates prefer YYYY-MM-DD (DD/MM/YYYY also ok).`
}

func (DocType) SchemaHint() string {
	return `{
  "numero": "string",
  "serie": "string",
  "emitente": { "nome": "string", "cnpj": "14 digits or empty", "cpf": "11 digits or empty" },
  "destinatario": { "nome": "string", "cnpj": "14 digits or empty", "cpf": "11 digits or empty" },
  "valor_total": 0,
  "data_emissao": "YYYY-MM-DD or null",
  "itens": [ { "descricao": "string", "quantidade": 0, "valor": 0 } ]
}`
}

func (DocType) EmptyResult() any {
	return &Result{Itens: []Item{}}
}

func (DocType) Normalize(result any) {
	r, ok := result.(*Result)
	if !ok || r == nil {
		return
	}
	r.Numero = strings.TrimSpace(r.Numero)
	r.Serie = strings.TrimSpace(r.Serie)
	r.Emitente = normalizeParty(r.Emitente)
	r.Destinatario = normalizeParty(r.Destinatario)
	r.DataEmissao = normalize.DateToISO(ptrString(r.DataEmissao))
	if r.Itens == nil {
		r.Itens = []Item{}
	}
}

func normalizeParty(p Party) Party {
	p.Nome = strings.TrimSpace(p.Nome)
	p.CNPJ = normalize.CNPJ(p.CNPJ)
	p.CPF = normalize.CPF(p.CPF)
	return p
}

func ptrString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
