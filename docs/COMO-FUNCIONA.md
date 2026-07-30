# Personal Document Extractor — Como funciona

Documento em português sobre o projeto: o que ele faz, por que as decisões foram tomadas assim, e como funcionam **autenticação**, **usuários** e **acesso máquina-a-máquina (M2M)**.

Contrato HTTP detalhado (status, erros, schemas): [`../openapi/openapi.yaml`](../openapi/openapi.yaml)  
README operacional (inglês): [`../README.md`](../README.md)  
IdP OIDC local (irmão): [`../../kalke-auth`](../../kalke-auth)

---

## 1. O que é este projeto?

API em **Go** que recebe um documento brasileiro (PDF ou imagem) e devolve **JSON estruturado**.

Tipos suportados (`doc_type`):

| Tipo | Exemplos |
|---|---|
| `identity_document` | RG, CNH, CNH-e |
| `address_proof` | Conta de luz, água, etc. |
| `invoice_nf` | NFe / NFSe (visão simplificada) |

Fluxo resumido:

1. Cliente autentica (`Bearer` com API key ou JWT OIDC).
2. Envia `multipart/form-data` com o arquivo + `doc_type`.
3. API valida o arquivo, prepara imagens (PDF → `pdftoppm`).
4. Chama um modelo de visão na **Groq**.
5. Normaliza campos (CPF, datas, UF…).
6. Responde só `{ "doc_type", "data" }` (sem metadados internos).
7. Em cache miss: grava Redis + Postgres (auditoria).

Stack:

- **Go** + **chi** (HTTP)
- **Postgres 18** (persistência / usuários / API keys)
- **Redis** (cache de extração + rate limit)
- **Groq** (LLM visão)
- **OIDC IdP** via repo **`kalke-auth`** (Keycloak encapsulado atrás de Caddy; opcional no ambiente local)
- **Docker Compose** + **Make** + **CI** (lint, testes, Postgres real, build)

---

## 2. Como a arquitetura se organiza

```
Cliente (curl / futuro site / SDK)
        │
        ▼
   API Go (chi)
   ├── /health, /ready          → públicos
   └── /v1/*                    → autenticados
        ├── /me                 → perfil do usuário local
        ├── /api-keys           → CRUD de keys do próprio usuário
        └── /extract            → extração (+ rate limit)
        │
        ├── AuthN/AuthZ
        ├── preprocess (PDF/imagem)
        ├── extract + doctypes
        ├── Groq
        ├── Redis (cache + rate limit)
        └── Postgres (users, api_keys, extractions)

Login humano (site futuro)
        │
        ▼
   kalke-auth (Caddy)  →  Keycloak (interno)  →  Postgres do IdP
```

Pacotes principais (`internal/`):

| Pacote | Responsabilidade |
|---|---|
| `httpapi` | Rotas, middleware, erros estáveis |
| `auth` / `authz` | Bearer, JWT via OIDC discovery/JWKS, scopes |
| `identity` | JWT → upsert na tabela `users` |
| `preprocess` | Validação MIME, compactação, PDF |
| `extract` + `doctypes/*` | Prompt, decode JSON, normalização |
| `cache` | Redis fail-open para resultado |
| `ratelimit` | Redis fail-closed por principal |
| `store` | Postgres (`users`, `api_keys`, `extractions`) |

---

## 3. Por que essas decisões?

### 3.1 Identidade humana no IdP OIDC (não senha no Postgres)

Queremos, no futuro próximo:

- login com **e-mail/senha**
- depois **Google / GitHub / Apple**
- depois **2FA / MFA**
- depois, eventualmente, **Web3**

Padrão de mercado (SaaS / CIAM):

- Um **IdP OIDC** cuida de senha, OAuth social, MFA, reset de senha.
- Nesta stack local, isso é o repo **`kalke-auth`**: Keycloak **só na rede Docker**, com **Caddy** como único rosto público (issuer limpo).
- A **API do produto** só valida o JWT (discovery → JWKS) e mantém uma linha local em `users` ligada ao `sub` do token.

A aplicação **não** configura `KEYCLOAK_*`. Ela conhece só:

```bash
OIDC_ISSUER=http://localhost:8443/realms/kalke
OIDC_AUDIENCE=personal-document-extractor
```

Caddy é **fachada de rede** (host/TLS/encapsulamento), não um BFF de auth em Go. A API continua validando JWT **localmente** com JWKS — não encaminha cada request ao IdP.

**Por que não guardar `password_hash` aqui?**  
Implementar OAuth social, MFA e recuperação de senha “na mão” aumenta risco e custo. O IdP entrega login; social/MFA viram configuração no realm, sem redesenhar schema do produto.

### 3.2 API keys para M2M (estilo Stripe)

`curl`, CI, backends e SDKs **não** fazem redirect OAuth. Por isso existe o segundo modo:

- chave `pde_live_…`
- só o **hash SHA-256** fica no banco
- prefixo público para lookup
- secret mostrado **uma vez**

Isso é o padrão de APIs de produto (Stripe, Twilio, OCR vendors). O `kalke-auth` **não** substitui API keys.

### 3.3 Separar AuthN e AuthZ

| Conceito | Pergunta | Artefato | Falha |
|---|---|---|---|
| **AuthN** | Quem é você? | JWT ou API key | `401` |
| **AuthZ** | O que pode fazer? | Scopes (`extract:write`, `keys:manage`, `admin`) | `403` |

Assim dá para trocar IdP (Keycloak → Cognito / Auth0 / Okta) sem reescrever regras de permissão — desde que o contrato OIDC (`issuer`, `audience`, `permissions`) se mantenha.

### 3.4 Cache Redis fail-open; rate limit fail-closed

- **Cache:** se Redis cair, a API continua extraindo (pior = mais custo/latência).
- **Rate limit:** se Redis cair no `/v1/extract`, responde `429` (protege quota Groq e abuso).

### 3.5 Resposta sem `meta`

O cliente recebe só o dado útil (`doc_type` + `data`). Modelo, cache hit/miss e hash ficam no servidor (logs + Postgres).

### 3.6 Soft-delete + `refresh=true`

Reextrair o mesmo arquivo:

- apaga chave Redis
- marca linha antiga com `deleted_at`
- grava nova extração

Histórico auditável sem perder o passado.

---

## 4. Como a autenticação funciona (visão geral)

Todo `/v1/*` exige:

```http
Authorization: Bearer <credencial>
```

A credencial pode ser:

1. **API key** começando com `pde_live_` → caminho M2M  
2. **JWT OIDC** (RS256) → caminho usuário humano (site futuro)

```
                    Authorization: Bearer …
                              │
                              ▼
                     ┌────────────────┐
                     │  Middleware    │
                     │  authenticate  │
                     └───────┬────────┘
                             │
           ┌─────────────────┴─────────────────┐
           │                                   │
    pde_live_…                          JWT (OIDC)
           │                                   │
           ▼                                   ▼
   hash + prefix no DB                  discovery + JWKS
   + user ativo?                        + audience/issuer
           │                            + upsert users
           └─────────────┬─────────────────────┘
                         ▼
                   Principal
                   (user_id, scopes)
                         │
                         ▼
              requireScope (quando aplicável)
                         │
                         ▼
                     Handler
```

`/health` e `/ready` são **públicos** (probes).

---

## 5. Como o usuário funciona

### 5.1 Dois “usuários” diferentes

| Camada | Onde vive | Papel |
|---|---|---|
| Identidade OIDC | `kalke-auth` (Keycloak) | Login, senha, social, MFA |
| Conta da aplicação | Tabela Postgres `users` | Dono de API keys e extractions |

A ponte é `users.auth_subject` = claim `sub` do JWT  
(ex.: UUID do Keycloak, ou `google-oauth2|…` em outros IdPs).

### 5.2 Tabela `users` (o que guardamos)

- `id` (UUID interno)
- `auth_subject` (único)
- `email`, `email_verified`, `display_name`
- `status` (`active` / `disabled`)
- timestamps / `last_login_at`

**Não** guardamos senha.

Também existe o usuário de bootstrap:

- `auth_subject = system:ops`
- usado por `make apikey` / `make admin` no ambiente local/ops

### 5.3 Upsert no JWT

Quando chega um JWT válido:

1. Resolve JWKS via `{issuer}/.well-known/openid-configuration`.
2. Valida assinatura, `issuer`, `audience`.
3. Lê `sub`, e se existirem `email` / `name` / `email_verified`.
4. Faz **upsert** em `users`.
5. Se `status != active` → `401`.
6. Monta o `Principal` com `UserID` + scopes.

Se o token **não** trouxer `permissions`, a API concede default:

- `extract:write`
- `keys:manage`

(útil no bootstrap; em produção dá para endurecer com roles no IdP — no `kalke-auth` as roles do realm já vão para o claim `permissions`.)

### 5.4 Endpoints de usuário

| Método | Path | Quem pode |
|---|---|---|
| `GET` | `/v1/me` | Qualquer principal autenticado com `user_id` (sem scope extra) |
| `GET` | `/v1/api-keys` | Scope `keys:manage` ou `admin` |
| `POST` | `/v1/api-keys` | Idem; cria key **para si** |
| `DELETE` | `/v1/api-keys/{id}` | Idem; revoga key **própria** |

Self-service cria keys com default `extract:write` (não `admin`).  
Admin de verdade fica para ops (`make admin` / scope `admin`).

### 5.5 Conta desabilitada

Usuário `disabled`:

- JWT: upsert não “reabilita”; acesso negado
- API key do dono: também negada
- `/v1/me`: `401`

---

## 6. Como o M2M funciona (API keys)

### 6.1 Formato da chave

Exemplo conceitual:

```text
pde_live_<prefixo_publico>_<segredo>
```

- **Prefixo** → coluna `key_prefix` (índice de busca)
- **Secret completo** → `SHA-256` em `key_hash` (comparação constant-time)
- Secret em texto claro **não** é armazenado

### 6.2 Ciclo de vida

**Criação (ops / local):**

```bash
make apikey NAME=local          # scope extract:write, dono system:ops
make admin                      # scope admin, dono system:ops
```

**Criação (usuário autenticado via JWT, futuro site):**

```http
POST /v1/api-keys
Authorization: Bearer <jwt>
Content-Type: application/json

{"name":"meu-backend","scopes":["extract:write"]}
```

Resposta `201` inclui `secret` **uma vez**.

**Uso:**

```bash
curl -X POST "http://localhost:8080/v1/extract?doc_type=identity_document" \
  -H "Authorization: Bearer pde_live_…" \
  -F "file=@./documento.pdf"
```

**Revogação:** `DELETE /v1/api-keys/{id}` → `revoked_at` (soft).

### 6.3 Ownership

Toda key tem `user_id` → FK para `users`.

- Keys do CLI → `system:ops`
- Keys do site (JWT) → usuário upsertado do IdP

Na extração, Postgres guarda também `api_key_id`, `auth_subject`, `user_id` (auditoria; **não** vai na resposta JSON).

### 6.4 Scopes (AuthZ)

| Scope | Permite |
|---|---|
| `extract:write` | `POST /v1/extract` |
| `keys:manage` | listar / criar / revogar **próprias** keys |
| `admin` | tudo (wildcard) |

---

## 7. Fluxo completo de uma extração

1. **AuthN** → Principal  
2. **AuthZ** → precisa `extract:write` (ou `admin`)  
3. **Rate limit** Redis (por principal; janela de 1 minuto; default 60/min)  
4. Lê upload (limite 32 MiB)  
5. `preprocess.Prepare` (MIME real, WebP/JPEG/PNG/PDF)  
6. Cache Redis `extract:v1:{doc_type}:{sha256(bytes)}`  
   - hit → devolve JSON (sem gravar Postgres de novo)  
   - miss → Groq → normalize → Redis SET + Postgres INSERT  
7. Resposta: `{ doc_type, data }`  
8. Headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining` (e `Retry-After` em 429)

Erros de cliente são **mensagens estáveis** (sem vazar stderr do `pdftoppm` / Groq). Catálogo: OpenAPI `ErrorCatalog`.

---

## 8. kalke-auth: o que configurar (humano)

Para o caminho JWT (site):

1. Subir o IdP: `make auth-up` (ou `make up-all`) — exige o sibling `../kalke-auth`
2. No extractor: `make setup-oidc` (escreve `OIDC_ISSUER` / `OIDC_AUDIENCE`)
3. Clients no realm `kalke`:
   - `personal-document-extractor` → **audience** da API
   - `kalke-spa` → futuro site (PKCE)
   - `kalke-cli` → **só smoke local** (password grant)
4. Demo: `demo@kalke.local` / `DemoPass123!`
5. Smoke (API no **host**):

```bash
make auth-up
make deps-up
make run          # outro terminal
make smoke-oidc
```

Neste repositório a API **não** tem `/login` nem `/register` com senha.  
Isso é intencional: login é do IdP (tema/Universal Login) + SDK do front.

Local sem IdP: só API keys (`make apikey` / `make admin`).

**Docker:** `localhost` dentro do container da API não é o host. Para JWT smoke, rode a API no host **ou** alinhe `KC_HOSTNAME` e `OIDC_ISSUER` a um nome alcançável dos dois lados.

---

## 9. O que fica de fora (roadmap consciente)

- UI do site / dashboard Next.js  
- Theme custom no Keycloak / social login  
- BFF Go na frente do IdP (desnecessário com OIDC puro)  
- Tabela `user_identities` para Web3 (quando for implementar SIWE)  
- OPA / SpiceDB (AuthZ fino multi-tenant)  
- Orgs / billing  

Essas peças encaixam no desenho atual sem jogar fora `users` + API keys.

---

## 10. Como falar disso em entrevista / portfólio

Pontos fortes para explicar:

1. **Dois modos de AuthN** (OIDC humano + API key M2M), um só header `Bearer`.  
2. **AuthZ por scopes**, separado do provedor de identidade.  
3. **IdP encapsulado** (`kalke-auth`: proxy + Keycloak interno); app só fala OIDC.  
4. **Segredo nunca em claro** (hash + prefix).  
5. **Fail-open vs fail-closed** conscientes (cache vs rate limit).  
6. **Contrato OpenAPI** com catálogo de erros.  
7. **CI** com Postgres real + migrations + lint.

Comando rápido para explorar:

```bash
make setup
make up-all          # IdP + API (ou make up só com API keys)
make admin
make me API_KEY='pde_live_…'
make extract API_KEY='pde_live_…' FILE=./documento.pdf

# JWT smoke (API no host):
# make setup-oidc && make auth-up && make deps-up && make run
# make smoke-oidc
```
