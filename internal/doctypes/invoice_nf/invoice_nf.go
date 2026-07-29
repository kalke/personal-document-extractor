package invoice_nf

const Name = "invoice_nf"

type Party struct {
	Nome string `json:"nome"`
	CNPJ string `json:"cnpj"`
	CPF  string `json:"cpf"`
}

type Item struct {
	Descricao string  `json:"descricao"`
	Quantidade float64 `json:"quantidade"`
	Valor     float64 `json:"valor"`
}

type Result struct {
	Numero       string  `json:"numero"`
	Serie        string  `json:"serie"`
	Emitente     Party   `json:"emitente"`
	Destinatario Party   `json:"destinatario"`
	ValorTotal   float64 `json:"valor_total"`
	DataEmissao  string  `json:"data_emissao"`
	Itens        []Item  `json:"itens"`
}

type DocType struct{}

func (DocType) Name() string { return Name }

func (DocType) SystemPrompt() string {
	return `You are a precise Brazilian document extraction engine.
Extract fields from a nota fiscal (NFe or NFSe). Prefer numeric amounts as JSON numbers.
Use empty string for unknown text fields, 0 for unknown numbers, and [] if no line items are found.
Dates as DD/MM/YYYY when possible.
Respond with JSON only.`
}

func (DocType) SchemaHint() string {
	return `{
  "numero": "string",
  "serie": "string",
  "emitente": { "nome": "string", "cnpj": "string", "cpf": "string" },
  "destinatario": { "nome": "string", "cnpj": "string", "cpf": "string" },
  "valor_total": 0,
  "data_emissao": "string",
  "itens": [ { "descricao": "string", "quantidade": 0, "valor": 0 } ]
}`
}

func (DocType) EmptyResult() any {
	return &Result{Itens: []Item{}}
}
