# Security

## Reporting

Email security concerns to the repository owner. Do not open public issues for active exploits or leaked credentials.

## Practices

- Secrets live only in gitignored env files (`prod.env`, `.env`) and CI/host secret stores — never in the repo.
- `POST /v1/extract` requires a valid Bearer token (OIDC JWT or Kalke PAT via introspection).
- Extract is rate-limited per principal (Redis, fail-closed).
- Raw uploaded files are not persisted; content hash, structured result, and audit metadata (subject, email, IP, user-agent) may be stored.
- LGPD extract consent (`lgpd-extract-v1`) is recorded before processing.

## CI scanners

Pull requests and `main` runs include:

- `golangci-lint`, `gosec`, `govulncheck`
- `gitleaks` (secret scan)
- `trivy fs` (filesystem advisories; non-blocking while noisy)

## Scope notes

Groq API keys, database URLs, Redis credentials, and `INTROSPECT_SECRET` must never be committed.
