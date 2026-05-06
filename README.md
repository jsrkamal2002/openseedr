# OpenSeedr

A self-hosted cloud torrent service — add magnet links or `.torrent` files, download them server-side, browse and download the resulting files directly to your browser, then delete them when done. Inspired by [seedr.cc](https://seedr.cc).

---

## Table of Contents

- [Features](#features)
- [Architecture](#architecture)
- [Tech Stack](#tech-stack)
- [Prerequisites](#prerequisites)
- [Quick Start (Docker Compose)](#quick-start-docker-compose)
- [Environment Variables](#environment-variables)
- [OAuth Setup](#oauth-setup)
- [API Reference](#api-reference)
- [Observability](#observability)
- [Kubernetes Deployment](#kubernetes-deployment)
- [Development Setup](#development-setup)
- [Project Structure](#project-structure)
- [Security](#security)
- [Roadmap](#roadmap)

---

## Features

- **Add torrents** via magnet link or `.torrent` file upload
- **Real-time progress** polling — see download speed, ETA, and completion percentage
- **File browser** — navigate directories, preview file sizes and dates
- **Direct download** — stream any file to your browser with a single click
- **Delete files** — remove torrents and their data when no longer needed
- **Per-user isolation** — every user's files are sandboxed in their own directory
- **Storage quota** — configurable per-user disk limit with a live usage bar
- **Authentication** — email/password + Google OAuth + GitHub OAuth
- **Pause / Resume** torrents at any time
- **Full observability** — distributed tracing (Jaeger), metrics (Prometheus + Grafana), structured logs (Loki), all wired through OpenTelemetry

---

## Architecture

```
Browser
  │
  ▼
Nginx  (reverse proxy, TLS termination)
  ├── /          → Frontend  (Vue 3 SPA, nginx)
  └── /api/      → API       (Go / Gin)
                      ├── PostgreSQL  (users, torrent records)
                      ├── qBittorrent (torrent engine, WebUI API)
                      └── /data/{userID}/  (shared volume, per-user dirs)

Observability sidecar
  API → OTLP/gRPC → OTel Collector
                        ├── Jaeger      (traces)
                        ├── Prometheus  (metrics scrape)
                        └── Loki        (logs)
  Grafana reads from all three.
```

---

## Tech Stack

| Layer | Technology |
|---|---|
| API | Go 1.25, Gin v1.12, GORM v1.25, JWT v5 |
| Auth | bcrypt, golang-jwt, Google + GitHub OAuth2 |
| Torrent engine | qBittorrent-nox (built from source) |
| Database | PostgreSQL 16 |
| Frontend | Vue 3.5, Vite 8, TypeScript 6, Pinia, Vue Router 4, Tailwind CSS 4, Axios |
| Observability | OpenTelemetry SDK v1.43 (traces + metrics + logs), Jaeger, Prometheus, Loki, Grafana |
| Container | Docker, Docker Compose v2 |
| Orchestration | Kubernetes (manifests in `k8s/`) |

---

## Prerequisites

| Tool | Minimum version | Notes |
|---|---|---|
| Docker | 24+ | with Compose plugin v2 |
| Docker Compose | 2.20+ | `docker compose` (not `docker-compose`) |
| Git | any | to clone repos |
| — | — | For k8s: kubectl 1.28+, a cluster with an RWX storage class |

No local Go or Node installation is required — the Dockerfiles handle everything.

---

## Quick Start (Docker Compose)

### 1. Clone the repositories

OpenSeedr uses the qBittorrent source repo as a sibling directory:

```bash
git clone https://github.com/your-org/openseedr.git
git clone https://github.com/qbittorrent/qBittorrent.git   # sibling of openseedr/
```

Expected layout:

```
repos/
├── openseedr/       ← this repo
└── qBittorrent/     ← qBittorrent source (built inside Docker)
```

### 2. Create your `.env` file

```bash
cd openseedr
cp .env.example .env
```

Edit `.env` and fill in **all required values** (see [Environment Variables](#environment-variables)).

### 3. Start the stack

```bash
docker compose up --build -d
```

First run compiles qBittorrent from source — this takes several minutes. Subsequent starts use the Docker layer cache and are fast.

### 4. Open the app

| Service | URL |
|---|---|
| OpenSeedr app | http://localhost |
| Grafana dashboards | http://localhost:3000 |
| Jaeger traces | http://localhost:16686 |
| Prometheus | http://localhost:9090 |
| qBittorrent WebUI | http://localhost:8081 *(internal, not exposed by default)* |

### 5. Register your first account

Go to http://localhost/register and create an account. The first registered user gets a 10 GB storage quota by default.

### 6. Add a torrent

Click **"Add torrent"** on the dashboard, paste a magnet link or upload a `.torrent` file, and watch the progress bar fill.

### 7. Download your files

Switch to the **Files** tab once the torrent completes, navigate to any file, and click the download arrow.

---

## Environment Variables

Create a `.env` file in the `openseedr/` directory. A `.env.example` is provided.

### Required

| Variable | Description |
|---|---|
| `DB_PASSWORD` | PostgreSQL password |
| `JWT_SECRET` | Secret used to sign JWTs — must be at least 32 characters |
| `QBT_PASS` | Password for the qBittorrent WebUI admin account |
| `GRAFANA_PASSWORD` | Grafana admin password |

### Optional (have defaults)

| Variable | Default | Description |
|---|---|---|
| `APP_ENV` | `production` | Set to `development` for debug logging |
| `DB_USER` | `openseedr` | PostgreSQL username |
| `DB_NAME` | `openseedr` | PostgreSQL database name |
| `QBT_USER` | `admin` | qBittorrent WebUI username |
| `CORS_ORIGIN` | `http://localhost` | Allowed frontend origin for CORS |
| `GRAFANA_USER` | `admin` | Grafana admin username |

### OAuth (optional — leave blank to disable)

| Variable | Description |
|---|---|
| `GOOGLE_CLIENT_ID` | Google OAuth2 app client ID |
| `GOOGLE_CLIENT_SECRET` | Google OAuth2 app client secret |
| `GOOGLE_REDIRECT_URL` | Must match the redirect URI registered in Google Cloud Console |
| `GITHUB_CLIENT_ID` | GitHub OAuth app client ID |
| `GITHUB_CLIENT_SECRET` | GitHub OAuth app client secret |
| `GITHUB_REDIRECT_URL` | Must match the callback URL registered in GitHub |

### Example `.env`

```dotenv
# Database
DB_PASSWORD=supersecretdbpassword

# JWT — generate with: openssl rand -hex 32
JWT_SECRET=replace_me_with_a_long_random_string_at_least_32_chars

# qBittorrent
QBT_USER=admin
QBT_PASS=qbt_admin_password

# Grafana
GRAFANA_USER=admin
GRAFANA_PASSWORD=grafana_admin_password

# OAuth (optional)
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=
GOOGLE_REDIRECT_URL=http://localhost/api/v1/auth/google/callback

GITHUB_CLIENT_ID=
GITHUB_CLIENT_SECRET=
GITHUB_REDIRECT_URL=http://localhost/api/v1/auth/github/callback
```

---

## OAuth Setup

### Google

1. Go to [Google Cloud Console](https://console.cloud.google.com/) → **APIs & Services** → **Credentials**
2. Create an **OAuth 2.0 Client ID** (type: Web application)
3. Add `http://localhost/api/v1/auth/google/callback` to **Authorised redirect URIs**
4. Copy the **Client ID** and **Client Secret** into `.env`

### GitHub

1. Go to **GitHub Settings** → **Developer settings** → **OAuth Apps** → **New OAuth App**
2. Set **Authorization callback URL** to `http://localhost/api/v1/auth/github/callback`
3. Copy the **Client ID** and generate a **Client Secret** into `.env`

For production, replace `http://localhost` with your domain in both the OAuth app settings and the `*_REDIRECT_URL` variables.

---

## API Reference

All endpoints are prefixed with `/api/v1`. Protected routes require:

```
Authorization: Bearer <jwt_token>
```

### Auth

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/auth/register` | — | Register with email + password |
| `POST` | `/auth/login` | — | Login, returns JWT |
| `GET` | `/auth/me` | ✓ | Current user profile |
| `GET` | `/auth/google` | — | Redirect to Google OAuth |
| `GET` | `/auth/google/callback` | — | Google OAuth callback |
| `GET` | `/auth/github` | — | Redirect to GitHub OAuth |
| `GET` | `/auth/github/callback` | — | GitHub OAuth callback |

**Register**
```bash
curl -X POST http://localhost/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","username":"yourname","password":"password123"}'
```

**Login**
```bash
curl -X POST http://localhost/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"password123"}'
# Response: { "token": "<jwt>", "user": { ... } }
```

### Torrents

| Method | Path | Description |
|---|---|---|
| `GET` | `/torrents` | List all torrents (with live progress sync) |
| `POST` | `/torrents/magnet` | Add a torrent by magnet link |
| `POST` | `/torrents/file` | Add a torrent by `.torrent` file upload |
| `GET` | `/torrents/:id` | Get a single torrent |
| `DELETE` | `/torrents/:id?delete_files=true` | Delete torrent (optionally delete files) |
| `POST` | `/torrents/:id/pause` | Pause downloading |
| `POST` | `/torrents/:id/resume` | Resume downloading |

**Add magnet**
```bash
curl -X POST http://localhost/api/v1/torrents/magnet \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"magnet_url":"magnet:?xt=urn:btih:..."}'
```

**Upload .torrent file**
```bash
curl -X POST http://localhost/api/v1/torrents/file \
  -H "Authorization: Bearer $TOKEN" \
  -F "torrent=@/path/to/file.torrent"
```

**Torrent object**
```json
{
  "id": "uuid",
  "name": "Ubuntu 24.04 LTS",
  "size": 2147483648,
  "downloaded": 1073741824,
  "progress": 0.5,
  "status": "downloading",
  "hash": "abc123...",
  "added_at": "2026-05-06T08:00:00Z"
}
```

Possible `status` values: `queued` `downloading` `seeding` `paused` `completed` `error`

### Files

| Method | Path | Description |
|---|---|---|
| `GET` | `/files?path=/` | List files/dirs at path |
| `GET` | `/files/download?path=/foo/bar.mkv` | Stream file download |
| `DELETE` | `/files?path=/foo/bar.mkv` | Delete file or directory |
| `GET` | `/files/storage` | Storage usage in bytes |

**List files**
```bash
curl "http://localhost/api/v1/files?path=/" \
  -H "Authorization: Bearer $TOKEN"
# Response: { "files": [...], "path": "/", "count": 3 }
```

**Download a file**
```bash
curl -OJ "http://localhost/api/v1/files/download?path=/Ubuntu+24.04/ubuntu.iso" \
  -H "Authorization: Bearer $TOKEN"
```

### Health & Metrics

| Method | Path | Description |
|---|---|---|
| `GET` | `/healthz` | Liveness check, returns `{"status":"ok"}` |
| `GET` | `/metrics` | Prometheus metrics scrape endpoint |

---

## Observability

OpenSeedr ships a full observability stack out of the box. Every API request is automatically instrumented.

### What is collected

| Signal | What | Where |
|---|---|---|
| **Traces** | Every HTTP request, DB query, qBittorrent call — with trace/span IDs | Jaeger |
| **Metrics** | Request rate, latency histograms, active requests, torrents added/deleted, storage used, login attempts | Prometheus → Grafana |
| **Logs** | Structured JSON with `trace_id` and `span_id` injected automatically | Loki → Grafana |

### Accessing the UI

```
Grafana   → http://localhost:3000   (admin / $GRAFANA_PASSWORD)
Jaeger    → http://localhost:16686
Prometheus→ http://localhost:9090
```

### Key Grafana dashboards

After first login to Grafana, the Prometheus, Loki, and Jaeger datasources are auto-provisioned. Import the following community dashboards by ID:

| Dashboard | Grafana ID | Description |
|---|---|---|
| Go runtime metrics | `13240` | Goroutines, GC, memory |
| Gin HTTP metrics | custom (use `openseedr_*` metrics) | Request rate, latency by route |
| PostgreSQL | `9628` | Query stats, connections |

### Correlating logs with traces

Every structured log line includes `trace_id` and `span_id` fields. In Grafana, open a log line in Loki and click the **Jaeger** link next to the trace ID to jump directly to the full request trace.

### Custom metrics exposed

| Metric | Type | Description |
|---|---|---|
| `openseedr_http_server_requests_total` | Counter | HTTP requests by method/route/status class |
| `openseedr_http_server_request_duration` | Histogram | Request latency in ms |
| `openseedr_http_server_active_requests` | Gauge | Requests currently in flight |
| `openseedr_torrents_added_total` | Counter | Torrents added per user |
| `openseedr_torrents_deleted_total` | Counter | Torrents deleted per user |
| `openseedr_torrents_active` | Gauge | Active (downloading/seeding) torrent count |
| `openseedr_torrents_downloaded_bytes` | Counter | Total bytes downloaded |
| `openseedr_storage_used_bytes` | Gauge | Disk usage per user |
| `openseedr_auth_login_attempts_total` | Counter | Login attempts by provider |
| `openseedr_auth_login_failures_total` | Counter | Failed login attempts by provider |
| `openseedr_auth_oauth_callbacks_total` | Counter | Completed OAuth flows |

---

## Kubernetes Deployment

All manifests are in `k8s/`. Apply them in order:

```bash
# 1. Create the namespace
kubectl apply -f k8s/namespace.yaml

# 2. Secrets — edit BEFORE applying, or use sealed-secrets
kubectl apply -f k8s/secrets.yaml

# 3. ConfigMap
kubectl apply -f k8s/configmap.yaml

# 4. Storage (PVCs) — adjust storageClassName to match your cluster
kubectl apply -f k8s/storage.yaml

# 5. Databases and engine
kubectl apply -f k8s/qbittorrent.yaml

# 6. API and frontend
kubectl apply -f k8s/api.yaml
kubectl apply -f k8s/frontend.yaml

# 7. Observability collector
kubectl apply -f k8s/observability.yaml

# 8. Ingress (edit host + TLS secret name first)
kubectl apply -f k8s/ingress.yaml
```

### Before applying

- **`k8s/secrets.yaml`** — replace all `changeme` values. In production, use [Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets) or [External Secrets Operator](https://external-secrets.io/) instead of committing plaintext secrets.
- **`k8s/storage.yaml`** — change `storageClassName: nfs` to a storage class that supports `ReadWriteMany` in your cluster (e.g., `efs-sc` on AWS, `azurefile` on Azure).
- **`k8s/ingress.yaml`** — replace `yourdomain.com` with your actual domain. The manifest assumes [cert-manager](https://cert-manager.io/) is installed for automatic TLS via Let's Encrypt.
- **`k8s/configmap.yaml`** — update `CORS_ORIGIN` and the OAuth redirect URLs to your domain.

### Verify the deployment

```bash
kubectl get pods -n openseedr
kubectl logs -n openseedr deploy/openseedr-api
kubectl get ingress -n openseedr
```

---

## Development Setup

For local development without Docker:

### API

```bash
# Prerequisites: Go 1.22+, PostgreSQL running locally

cd api
cp ../.env.example .env
# Edit .env — set DB_HOST=localhost, QBITTORRENT_URL=http://localhost:8080, etc.

go mod download
go run .
# API listens on :8080
```

### Frontend

```bash
cd frontend
npm install
npm run dev
# Dev server on http://localhost:5173
# Requests to /api/* are proxied to http://localhost:8080 (see vite.config.ts)
```

### qBittorrent (local, no build)

The easiest option for dev is to use the pre-built Docker image:

```bash
docker run -d \
  -p 8080:8080 \
  -p 6881:6881 \
  -v qbt-config:/config \
  -v qbt-data:/data \
  linuxserver/qbittorrent
```

Then set `QBITTORRENT_URL=http://localhost:8080` in your `.env`.

### Lint & security checks

```bash
# From api/
go vet ./...
staticcheck ./...        # go install honnef.co/go/tools/cmd/staticcheck@latest
gosec ./...              # go install github.com/securego/gosec/v2/cmd/gosec@latest

# From frontend/
npm run build            # runs vue-tsc type check + vite build
```

---

## Project Structure

```
openseedr/
├── api/                        Go API server
│   ├── main.go                 Entry point, router setup, graceful shutdown
│   ├── go.mod / go.sum
│   ├── Dockerfile              Multi-stage, scratch final image, non-root
│   ├── db/
│   │   └── db.go               GORM connection + AutoMigrate
│   ├── handlers/
│   │   ├── auth.go             Register, Login, OAuth (Google/GitHub)
│   │   ├── torrents.go         CRUD + pause/resume
│   │   └── files.go            List, download, delete, storage info
│   ├── middleware/
│   │   └── auth.go             JWT Bearer guard
│   ├── models/
│   │   ├── user.go
│   │   └── torrent.go
│   ├── observability/
│   │   ├── telemetry.go        OTel SDK init (tracer, meter, logger providers)
│   │   ├── metrics.go          Custom metric instruments + helpers
│   │   ├── middleware.go       Gin HTTP middleware (spans, metrics, access log)
│   │   └── helpers.go          GetEnvOrDefault
│   └── services/
│       ├── auth.go             bcrypt, JWT sign/verify
│       ├── oauth.go            Google + GitHub user info exchange
│       ├── qbittorrent.go      qBittorrent Web API client
│       └── storage.go          os.Root-based traversal-safe file ops
│
├── frontend/                   Vue 3 + TypeScript SPA
│   ├── src/
│   │   ├── main.ts
│   │   ├── App.vue
│   │   ├── router/index.ts     Vue Router (auth guard)
│   │   ├── composables/
│   │   │   └── useApi.ts       Axios instance + interceptors
│   │   ├── stores/
│   │   │   ├── auth.ts         Pinia — user session
│   │   │   ├── torrents.ts     Pinia — torrent list
│   │   │   └── files.ts        Pinia — file browser
│   │   ├── views/
│   │   │   ├── LoginView.vue
│   │   │   ├── RegisterView.vue
│   │   │   ├── DashboardView.vue
│   │   │   └── FilesView.vue
│   │   └── components/
│   │       ├── AppLayout.vue   Sidebar + main area shell
│   │       ├── TorrentCard.vue Progress bar, pause/resume/delete
│   │       ├── FileBrowser.vue Directory table, download/delete
│   │       ├── AddTorrentModal.vue Magnet / file upload tabs
│   │       └── StorageBar.vue  Quota usage bar
│   ├── nginx.conf              Hardened nginx config for SPA serving
│   └── Dockerfile              Multi-stage, non-root nginx
│
├── qbittorrent/
│   └── Dockerfile              Builds qBittorrent-nox from source
│
├── otel-collector/
│   └── config.yaml             Receives OTLP → exports to Jaeger, Prometheus, Loki
│
├── prometheus/
│   └── prometheus.yml          Scrape config for api + otel-collector
│
├── grafana/
│   └── provisioning/
│       └── datasources/        Auto-provisioned Prometheus, Loki, Jaeger
│
├── nginx/
│   └── nginx.conf              Reverse proxy: routes /api → api, / → frontend
│
├── k8s/                        Kubernetes manifests
│   ├── namespace.yaml
│   ├── secrets.yaml
│   ├── configmap.yaml
│   ├── storage.yaml            PVCs + PostgreSQL StatefulSet
│   ├── api.yaml                Deployment + Service + HPA
│   ├── frontend.yaml           Deployment + Service
│   ├── qbittorrent.yaml        StatefulSet + Service
│   ├── observability.yaml      OTel Collector Deployment + ConfigMap
│   └── ingress.yaml            Nginx ingress + cert-manager TLS
│
└── docker-compose.yml          Full local stack
```

---

## Security

### What is hardened

| Area | Measures |
|---|---|
| **Container images** | API image uses `scratch` (no shell, no OS tools). All images run as non-root users. |
| **Filesystem** | API container runs with `read_only: true` in Compose; Kubernetes pods set `readOnlyRootFilesystem: true`. |
| **Capabilities** | All Linux capabilities dropped (`drop: [ALL]`) in Kubernetes pods. |
| **File access** | Storage service uses `os.Root` (Go 1.24+) which prevents directory-traversal at the OS level — no path can escape a user's directory. |
| **Directory permissions** | User storage directories are created with `0750` (no world access). |
| **Passwords** | Stored as bcrypt hashes (cost 10). Never logged or returned in API responses. |
| **JWT** | Signed with HS256, 7-day expiry. Secret loaded from environment variable, never hardcoded. |
| **OAuth CSRF** | State parameter is generated per-request, stored in a `Secure; HttpOnly; SameSite=Strict` cookie, and verified on callback. |
| **HTTP headers** | Nginx sets `X-Frame-Options`, `X-Content-Type-Options`, `X-XSS-Protection`, `Referrer-Policy`, `Permissions-Policy`, `Content-Security-Policy`. |
| **Input validation** | Gin `binding:"required,email"` tags + explicit field length limits on all request bodies. |
| **Torrent file size** | `.torrent` file uploads are capped at 10 MB server-side. |
| **Static analysis** | `go vet`, `staticcheck`, and `gosec` all pass with 0 findings. |

### Recommendations for production

- Put `k8s/secrets.yaml` behind [Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets) or External Secrets — never commit plaintext secrets to git.
- Enable TLS everywhere: use cert-manager + Let's Encrypt (ingress manifest includes the annotations).
- Rotate `JWT_SECRET` periodically and set a short expiry for sensitive deployments.
- Restrict qBittorrent's WebUI to the internal network — do not expose port 8080 publicly.
- Set per-user storage quotas appropriate for your disk size.

---

## Roadmap

- [ ] Email verification on registration
- [ ] Admin panel (manage users, quotas, view all torrents)
- [ ] Stripe integration for paid storage tiers
- [ ] WebSocket for real-time torrent progress (replace polling)
- [ ] ZIP download for entire torrent directories
- [ ] Dark / light theme toggle
- [ ] Torrent search (via Jackett / Prowlarr integration)
- [ ] Two-factor authentication

---

## License

MIT — see [LICENSE](LICENSE).
