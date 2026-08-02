# Deploy

Push to `main` runs **Lint → Test → Security → Build → Deploy**.

Production API runs on the **same AWS EC2** as `kalke-auth` (Neon + Upstash), behind Caddy at `pde.kalke.dev`.

> Cloudflare Containers need the **Workers Paid** plan. Until then, CI deploys to EC2 via a self-hosted runner (`pde-ec2`).

## One-time

1. Self-hosted runner on the EC2 host (label `pde-ec2`) — already used by CI.
2. DNS: Cloudflare **A** `pde` → EC2 Elastic IP, **DNS only** (grey cloud), same as `auth`.
3. `kalke-auth` Caddyfile must proxy `pde.kalke.dev` → `pde-api:8080` (in repo).

## GitHub Actions secrets

- `DATABASE_URL`, `REDIS_ADDR`, `REDIS_PASSWORD`
- `OIDC_ISSUER`, `INTROSPECT_SECRET`, `GROQ_API_KEY`
- (optional, for future CF Containers) `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`

Deploy writes `~/personal-document-extractor/prod.env` from these secrets, then `make aws-up`.

## Manual update

```bash
ssh -i first.pem ubuntu@EIP
cd ~/personal-document-extractor
# needs GH_TOKEN for private pull, or git pull if public
make aws-up
```
