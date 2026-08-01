# Deploy personal-document-extractor (pde.kalke.dev)

Cloudflare Containers + Neon Postgres + Upstash Redis.

## 1. Neon (free)

Create database `pde`. Connection string example:

```text
postgres://user:pass@ep-xxx.region.aws.neon.tech/pde?sslmode=require
```

→ GitHub / Wrangler secret `DATABASE_URL`.

## 2. Upstash Redis (free)

Create a Redis DB. From the dashboard:

| Secret | Value |
|---|---|
| `REDIS_ADDR` | `friendly-name.upstash.io:6379` |
| `REDIS_PASSWORD` | Upstash password |
| `REDIS_TLS` | `true` (set as Wrangler var already) |

## 3. OIDC + Groq

| Secret | Value |
|---|---|
| `OIDC_ISSUER` | `https://auth.kalke.dev/realms/kalke` |
| `OIDC_AUDIENCE` | `personal-document-extractor` |
| `GROQ_API_KEY` | Groq API key |

Deploy [kalke-auth](https://github.com/kalke/kalke-auth) first.

## 4. GitHub secrets

Also set `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID`.

## 5. Deploy

Push to `main` after PR merge, or:

```bash
npm ci
npx wrangler secret put DATABASE_URL
npx wrangler secret put REDIS_ADDR
npx wrangler secret put REDIS_PASSWORD
npx wrangler secret put OIDC_ISSUER
npx wrangler secret put GROQ_API_KEY
npm run deploy
```

## 6. Branch protection

Require PR + checks (`Lint and test`, `Docker build`) and restrict `main` pushes to `kalke`.
