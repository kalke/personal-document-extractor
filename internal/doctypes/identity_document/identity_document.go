package identity_document

const Name = "identity_document"

type Result struct {
	Tipo           string `json:"tipo"`
	Nome           string `json:"nome"`
	CPF            string `json:"cpf"`
	RGOuRegistro   string `json:"rg_ou_registro"`
	DataNascimento string `json:"data_nascimento"`
	OrgaoEmissor   string `json:"orgao_emissor"`
	Validade       string `json:"validade"`
}

type DocType struct{}

func (DocType) Name() string { return Name }

func (DocType) SystemPrompt() string {
	return `You are a precise Brazilian document extraction engine.
Extract fields from an identity document (RG or CNH).
Set "tipo" to "RG", "CNH", or "outro".
Use empty string for unknown fields. Dates as DD/MM/YYYY when possible. CPF as ###.###.###-## when possible.
Respond with JSON only.`
}

func (DocType) SchemaHint() string {
	return `{
  "tipo": "string — RG | CNH | outro",
  "nome": "string — full legal name",
  "cpf": "string — CPF number",
  "rg_ou_registro": "string — RG number or CNH registry number",
  "data_nascimento": "string — birth date",
  "orgao_emissor": "string — issuing authority",
  "validade": "string — expiration date if present"
}`
}

func (DocType) EmptyResult() any { return &Result{} }
