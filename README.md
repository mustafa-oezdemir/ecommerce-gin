# Ecommerce Gin

A secure, server-rendered e-commerce demo built with Go, Gin, GORM, and MySQL. It includes customer shopping flows, operational tooling for employees, and an admin back office—ready to run as a local Docker stack.

## Highlights

- **Customer experience** — browse products, manage a cart, complete checkout, and view orders.
- **Operations** — manage products, inventory, and order status as an employee.
- **Administration** — review dashboards, users, categories, and orders.
- **Security by default** — signed sessions, RBAC, ownership checks, CSRF protection, secure headers, validated requests, and rate-limited sign-in.
- **Safe product media** — employee image uploads are size-limited, virus-scanned, decoded, sanitized, and stored under generated names.
- **Reliable commerce data** — integer-cent pricing, transactional checkout, immutable order item snapshots, foreign keys, and indexed migrations.
- **Observability** — liveness/readiness probes, structured request logs, Prometheus metrics, and a provisioned Grafana dashboard.

## Stack

| Area | Technology |
| --- | --- |
| Application | Go, Gin, GORM |
| Database | MySQL 8 |
| UI | Server-rendered HTML templates |
| Local platform | Docker Compose, MailHog |
| Monitoring | Prometheus, Grafana |
| Upload security | ClamAV (`clamd`) and image re-encoding |

## Roles

| Role | Capabilities |
| --- | --- |
| Customer | Browse products, manage own cart, checkout, update own profile/password, see own orders |
| Employee | Operational dashboard, products, inventory and order status transitions |
| Admin | Employee capabilities plus dashboards, users and category management |

## Quick start

### 1. Configure the environment

```bash
cp .env.example .env
```

Set unique local values for `MYSQL_*`, `SESSION_SECRET`, `CSRF_SECRET`, and Grafana credentials. `SESSION_SECRET` must contain at least 32 characters; `CSRF_SECRET` must be a base64-encoded 32-byte value.

### 2. Start the stack

```bash
docker compose up --build -d
docker compose run --rm app /app/seed
```

### 3. Open the services

| Service | Address |
| --- | --- |
| Storefront | http://localhost:8080 |
| MailHog | http://localhost:8025 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3000 |

Seed users exist only for development/test:

| Role | Email | Password |
| --- | --- | --- |
| Admin | `admin@example.com` | `AdminPass123!` |
| Employee | `employee@example.com` | `EmployeePass123!` |
| Customer | `customer@example.com` | `CustomerPass123!` |

Never use these seed accounts or their passwords in production. The seed command refuses to run when `APP_ENV=production`.

## Application areas

| Area | Routes | Access |
| --- | --- | --- |
| Shop | `/`, `/products`, `/cart`, `/checkout` | Customer actions require sign-in |
| Account | `/account`, `/account/orders` | Signed-in customer |
| Employee | `/employee/*` | Employee or admin |
| Admin | `/admin/*` | Admin only |
| Health | `/health/live`, `/health/ready` (`/healthz`, `/readyz` aliases) | Public |

## Configuration

Copy `.env.example` and keep `.env` private. Docker passes only application-required values to the app container; the MySQL root password is not exposed to it. `SMTP_*` targets MailHog only and is disabled by the application in production.

| Variable | Purpose |
| --- | --- |
| `APP_ENV` | `development`, `test`, or `production` |
| `TRUSTED_PROXIES` | Comma-separated proxy IPs/CIDRs; leave empty for direct traffic |
| `APP_PORT` | Application HTTP port |
| `METRICS_PORT` | Internal Prometheus metrics port |
| `LOG_LEVEL` | Minimum log level: `debug`, `info`, `warn`, or `error` |
| `LOG_CONSOLE_FORMAT` | Console output format: `text` or `json` |
| `LOG_FILE` | Rotating application log path; Docker uses `/app/logs/ecommerce.log` |
| `LOG_MAX_SIZE_MB`, `LOG_MAX_BACKUPS`, `LOG_MAX_AGE_DAYS` | File rotation and retention limits |
| `LOG_COMPRESS`, `LOG_ADD_SOURCE` | Compress rotated files and optionally include source locations |
| `HTTP_READ_HEADER_TIMEOUT`, `HTTP_READ_TIMEOUT`, `HTTP_WRITE_TIMEOUT`, `HTTP_IDLE_TIMEOUT` | Public and metrics server connection timeouts |
| `HTTP_SHUTDOWN_TIMEOUT` | Maximum graceful-shutdown duration before connections are forced closed |
| `HTTP_MAX_HEADER_BYTES` | Maximum accepted HTTP request-header size |
| `PRODUCT_IMAGE_DIRECTORY` | Private storage directory for sanitized product images |
| `PRODUCT_IMAGE_MAX_BYTES` | Maximum uploaded and sanitized image size in bytes |
| `PRODUCT_IMAGE_MAX_WIDTH`, `PRODUCT_IMAGE_MAX_HEIGHT`, `PRODUCT_IMAGE_MAX_PIXELS` | Decoded-image limits that prevent image bombs |
| `CLAMAV_ADDRESS`, `CLAMAV_SCAN_TIMEOUT` | Internal `clamd` endpoint and fail-closed scan timeout |
| `MYSQL_*` | MySQL connection settings |
| `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS` | Database connection pool limits |
| `DB_CONN_MAX_LIFETIME`, `DB_CONN_MAX_IDLE_TIME` | Connection rotation and idle limits |
| `DB_CONNECT_TIMEOUT`, `DB_READ_TIMEOUT`, `DB_WRITE_TIMEOUT`, `DB_PING_TIMEOUT` | Database network and startup health timeouts |
| `SESSION_SECRET` | Cookie-session signing secret |
| `SESSION_SECURE` | Set to `true` in production |
| `CSRF_SECRET` | Base64-encoded 32-byte CSRF key |

Use `MYSQL_HOST=127.0.0.1` and `MYSQL_PORT=3307` for a host process; the Docker application receives `MYSQL_HOST=mysql` and port `3306`.

## Database

`migrations/000001_initial.sql` is applied once and tracked in `schema_migrations`. It defines indexes, foreign keys, user-email uniqueness, integer-cent money columns and cart/order ownership relationships.

## Health checks

- `GET /health/live` checks whether the process is running.
- `GET /health/ready` verifies database connectivity.
- Prometheus exports both results as `ecommerce_health_live` and `ecommerce_health_ready`; Grafana displays them on the overview dashboard.

Neither endpoint exposes configuration or secrets.

```bash
curl http://localhost:8080/health/live
curl http://localhost:8080/health/ready
```

## Monitoring

The application exposes Prometheus metrics only on its internal Docker port `9091`; it is scraped by Prometheus as `app:9091` and is not published by Compose. Grafana is provisioned automatically with the Prometheus datasource and an **Ecommerce Overview** dashboard. Metric labels use Gin route templates rather than raw URLs and never include customer or secret values.

## Middleware

Global middleware is installed as one ordered stack: request ID, structured access logging, Prometheus metrics, security headers, panic recovery, and sessions. Access logs use route templates and intentionally omit query strings; successful health-probe logs are skipped to reduce noise, while failures are always logged. Development uses readable text logs and production emits JSON to standard output so the runtime platform can collect and rotate them.

Gin does not trust forwarded client IP headers by default. Set `TRUSTED_PROXIES` only when traffic arrives through known proxy IPs or CIDR ranges; this keeps login rate limiting tied to the real client address without trusting spoofed headers.

## Logging

The application uses one `log/slog` pipeline for HTTP requests, Gin diagnostics, database warnings, application events, and recovered panics. Records are written to the console and to a rotating file. The file is always newline-delimited JSON; the development console defaults to text and production defaults to JSON. Request status controls its level: 2xx/3xx is `INFO`, 4xx is `WARN`, and 5xx is `ERROR`. Raw query strings, passwords, tokens, and session values are not recorded.

Administrators can inspect the structured log at `/admin/logs`. The dashboard supports level and text filters, bounded result sizes, newest-first entries, file statistics, and optional 10-second refreshes. It reads only the newest 2 MB instead of loading the entire rotating file and applies an additional sensitive-field redaction pass before rendering values as escaped HTML.

Docker stores logs in the persistent `app_logs` volume. Inspect the active file with:

```bash
docker compose exec app tail -f /app/logs/ecommerce.log
```

For a host process, the default file is `logs/ecommerce.log`. Rotation defaults to 100 MB per file, five backups, 28 days of retention, and gzip compression.

## HTTP server architecture

Router composition and HTTP process management live in `internal/server`, leaving `cmd/server` as the application composition root. The application and metrics listeners start as one supervised group: if either listener fails, both shut down. `SIGINT` and `SIGTERM` stop accepting new connections, allow active requests to finish within the configured shutdown timeout, and then force-close remaining connections.

Both listeners enforce explicit read-header, read, write, idle, and maximum-header limits. Request contexts propagate to database operations, and database/log resources close after the listeners have stopped.

## Product images

Employees can add an optional image while creating a product or replace an image from Product Management. Only JPEG (`.jpg`, `.jpeg`) and PNG (`.png`) files are accepted. The server enforces the request and file size before processing, compares the extension with detected content and the decoder format, checks dimensions and total pixels, streams the original bytes to ClamAV, and fails closed when the scanner cannot be reached.

Accepted images are decoded and re-encoded before storage. This removes original metadata, trailing payloads, and the client filename. A random server-generated filename is stored in MySQL, while the sanitized file is held in the persistent `app_uploads` Docker volume with non-executable permissions. Replaced files and failed database writes are cleaned up automatically. Public image responses allow only generated filenames and use immutable caching.

ClamAV is reachable only on the Compose network; port `3310` is not published to the host. Its signature database is retained in the `clamav_data` volume and updated by the official container.

## Development

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
go build ./...
docker compose config
```

CI runs formatting, vetting, tests, race detection, and builds. The security workflow runs `govulncheck` and `gosec`; Dependabot tracks Go, Docker and GitHub Actions updates.

## Production notes

Run behind an HTTPS reverse proxy and set `APP_ENV=production`, `GIN_MODE=release` and `SESSION_SECURE=true`. Supply unique secrets through a secrets manager, keep MySQL on a private network, configure backups/monitoring, and use a real production mail provider only through a separately reviewed integration. MailHog is intentionally development/test only.
