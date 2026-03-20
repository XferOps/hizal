# Self-Hosting Hizal

## Requirements
- Docker + Docker Compose v2
- Linux VPS (1 vCPU, 1 GB RAM minimum; 2 vCPU / 2 GB recommended)
- OpenAI API key (for embeddings)
- A domain with DNS pointed at your server (for SSL)
- SMTP credentials for transactional email

## Quick Start

1. Clone the repo (or download `docker-compose.prod.yml` and `.env.prod.example` directly):
   ```bash
   git clone https://github.com/parkerscobey/hizal.git
   cd hizal
   ```

2. Copy the example env file and fill in your values:
   ```bash
   cp .env.prod.example .env
   nano .env  # or your editor of choice
   ```

3. Start:
   ```bash
   docker compose -f docker-compose.prod.yml up -d
   ```

4. Verify:
   ```bash
   curl http://localhost:8080/health
   # {"status":"ok","version":"x.x.x"}
   ```

## Reverse Proxy

Put Hizal behind a reverse proxy for SSL termination.

**Caddy (recommended)** — handles SSL automatically via Let's Encrypt:
```
your-domain.com {
    reverse_proxy localhost:8080
}
```
Run: `caddy run --config Caddyfile`

**nginx:**
```nginx
server {
    listen 80;
    server_name your-domain.com;
    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```
See nginx docs for SSL setup with certbot.

## Upgrades

Migrations run automatically on startup. To upgrade:
```bash
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
```

## Backups

Dump your Postgres data:
```bash
docker exec hizal-postgres pg_dump -U hizal hizal > hizal-backup-$(date +%Y%m%d).sql
```
Schedule with cron. Restore with `psql`.

## What's Not Included

Self-hosting gives you the API. It does not include:
- The [Hizal UI](https://github.com/XferOps/hizal-ui) (deploy separately or use the hosted version)
- Automatic upgrade notifications
- Usage analytics or telemetry dashboards
- Managed backups

For teams that want all of this handled, the hosted version at [hizal.xferops.dev](https://hizal.xferops.dev) includes everything.