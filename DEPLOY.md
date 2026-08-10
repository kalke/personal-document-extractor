# Deploy

Push to `main` runs **Lint → Test → Security → Build → Deploy**.

Production API runs on the **same AWS EC2** as `kalke-auth` (Neon + Upstash), behind Caddy at `pde.kalke.dev`.

## One-time

1. Self-hosted runner on the EC2 host (label `pde-ec2`) — already used by CI.
2. DNS: Cloudflare **A** `pde` → Elastic IP `54.234.95.66`, **DNS only** (grey cloud), same as `auth`.
3. `kalke-auth` Caddyfile must proxy `pde.kalke.dev` → `pde-api:8080` (in repo).

## GitHub Actions secrets

- `DATABASE_URL`, `REDIS_ADDR`, `REDIS_PASSWORD`
- `OIDC_ISSUER`, `INTROSPECT_SECRET`, `GROQ_API_KEY`
- `M2M_USER_FORWARD_SECRET` (shared with Auth BFF `PDE_USER_FORWARD_SECRET`)
- `CLOUDFLARE_API_TOKEN` (DNS upsert via `dns-pde` workflow only)

Deploy writes `~/personal-document-extractor/prod.env` from these secrets, then `make aws-up`.

## Manual update

```bash
ssh ubuntu@54.234.95.66
cd ~/personal-document-extractor
make aws-up
```
