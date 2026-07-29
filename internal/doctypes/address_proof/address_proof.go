package address_proof

const Name = "address_proof"

type Result struct {
	Nome       string `json:"nome"`
	Logradouro string `json:"logradouro"`
	Numero     string `json:"numero"`
	Bairro     string `json:"bairro"`
	Cidade     string `json:"cidade"`
	UF         string `json:"uf"`
	CEP        string `json:"cep"`
	Emissor    string `json:"emissor"`
	Data       string `json:"data"`
}

type DocType struct{}

func (DocType) Name() string { return Name }

func (DocType) SystemPrompt() string {
	return `You are a precise Brazilian document extraction engine.
Extract fields from a comprovante de endereço (utility bill, bank statement, or residence declaration).
Use empty string for unknown fields. Dates as DD/MM/YYYY when possible. CEP as #####-### when possible.
Respond with JSON only.`
}

func (DocType) SchemaHint() string {
	return `{
  "nome": "string — full name of the resident/holder",
  "logradouro": "string — street name",
  "numero": "string — street number",
  "bairro": "string — neighborhood",
  "cidade": "string — city",
  "uf": "string — 2-letter state code",
  "cep": "string — postal code",
  "emissor": "string — issuer (company or authority)",
  "data": "string — document date"
}`
}

func (DocType) EmptyResult() any { return &Result{} }
