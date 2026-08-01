# Deploy

Push to `main` deploys via GitHub Actions → Cloudflare Containers (`pde.kalke.dev`).

Required GitHub Actions secret **names** (never commit values):

- `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`
- `DATABASE_URL`, `REDIS_ADDR`, `REDIS_PASSWORD`
- `OIDC_ISSUER`, `INTROSPECT_SECRET`, `GROQ_API_KEY`

`AUTH_INTROSPECT_URL` is set in `wrangler.json` vars. Deploy after `kalke-auth`.
