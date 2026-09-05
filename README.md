# Ecommerce Gin

A secure, server-rendered e-commerce demo built with Go, Gin, GORM, and MySQL. It includes customer shopping flows, operational tooling for employees, and an admin back office—ready to run as a local Docker stack.

## Highlights

- **Customer experience** — browse products, manage a cart, complete checkout, and view orders.
- **Product engagement** — reusable favorites, personal product lists, 1–10 ratings, and verified-purchase reviews.
- **Account security** — email verification, TOTP two-factor authentication, single-use recovery codes, and security-versioned sessions.
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
| Customer | Browse products, manage own cart, favorites and lists, checkout, review verified purchases, manage account security, see own orders |
| Employee | Operational dashboard, products, inventory and order status transitions |
| Admin | Employee capabilities plus dashboards, users and category management |

## Quick start

### 1. Configure the environment

```bash
cp .env.example .env
```

Set unique local values for `MYSQL_*`, `SESSION_SECRET`, `CSRF_SECRET`, `SECURITY_ENCRYPTION_KEY`, and Grafana credentials. `SESSION_SECRET` must contain at least 32 characters; both cryptographic keys must be independent base64-encoded 32-byte values. Development can derive a compatibility key from `CSRF_SECRET`, but production refuses to start without the dedicated key.

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
| Account security | `/account`, `/account/two-factor` | Any signed-in user |
| Customer account | `/account/orders`, `/account/lists` | Signed-in customer |
| Product engagement | `/products/:id/favorite`, `/products/:id/lists`, `/products/:id/reviews`, `/reviews/:id` | Signed-in customer; JSON/AJAX |
| Two-factor challenge | `/auth/two-factor-challenge` | Password-verified session awaiting TOTP/recovery code |
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
| `SECURITY_ENCRYPTION_KEY` | Independent base64-encoded 32-byte AES/HMAC key for TOTP secrets and one-time code hashes; mandatory in production |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_FROM` | Outbound email endpoint and sender; Compose points these to MailHog |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | Production SMTP credentials; required before production email delivery is enabled |

Use `MYSQL_HOST=127.0.0.1` and `MYSQL_PORT=3307` for a host process; the Docker application receives `MYSQL_HOST=mysql` and port `3306`.

## Database

SQL migrations are applied once and tracked in `schema_migrations`. `000004_account_security.sql` adds normalized recovery-code and pending-email-verification records plus encrypted TOTP/session-security fields. `000005_favorites_reviews.sql` adds a protected system-list key for Favorites and product reviews with database-enforced one-review-per-user/product and 1–10 rating constraints. `000006_account_email_length.sql` aligns the email column with the validated 254-character limit.

The database is opened once in `cmd/server` and passed explicitly through the router to handlers, services, and repositories. There is no package-level database singleton or runtime type assertion. Every request-path query uses the request context, while checkout and order-status changes keep their row locks and writes inside GORM transactions. The underlying `database/sql` pool is configured with bounded open/idle connections, connection lifetime, idle timeout, startup ping timeout, and graceful shutdown.

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

Employees can add up to eight images to each product, either while creating it or later from Product Management. They can choose the storefront cover image and delete individual gallery images. Only JPEG (`.jpg`, `.jpeg`) and PNG (`.png`) files are accepted. The server enforces per-file and request size limits before processing, compares each extension with detected content and the decoder format, checks dimensions and total pixels, streams every original file to ClamAV, and fails closed when the scanner cannot be reached.

Accepted images are decoded and re-encoded before storage. This removes original metadata, trailing payloads, and the client filename. Random server-generated filenames and their stable gallery order are stored in MySQL, while sanitized files are held in the persistent `app_uploads` Docker volume with non-executable permissions. Deleted files and failed database writes are cleaned up automatically. Public image responses allow only generated filenames and use immutable caching. The legacy single-image value is migrated automatically and remains the cover-image reference for backward compatibility.

ClamAV is reachable only on the Compose network; port `3310` is not published to the host. Its signature database is retained in the `clamav_data` volume and updated by the official container.

## Product API

The public read API is versioned under `/api/v1/products`. Collection and detail resources support `GET`, `HEAD`, and `OPTIONS`, use a consistent JSON success/error envelope, and never expose database model internals.

The collection endpoint supports bounded limit/offset pagination plus `category`, `min_price`, `max_price`, `q`, `sort`, and `order` query parameters. Sort fields and directions are allow-listed before reaching the repository, and every query uses the request context and parameterized GORM conditions.

Example:

```text
GET /api/v1/products?limit=20&offset=0&category=2&min_price=10&max_price=250&q=phone&sort=price&order=asc
```

## Account and product security flows

Account profile changes keep email updates separate. An email change requires the current password, sends a cryptographically random eight-digit code to the new address, stores only its keyed hash, expires after ten minutes, limits attempts, and applies a resend cooldown. Password, email, 2FA, and recovery-code changes increment the account security version so other signed sessions stop working.

TOTP setup uses a standards-based authenticator QR code. The secret is encrypted with AES-256-GCM at rest. Enabling 2FA creates eight readable, single-use recovery codes; only their HMAC-SHA-256 hashes are stored. Setup responses are marked `no-store`, login challenges expire after five minutes, and sensitive endpoints are CSRF protected and rate limited. Security logs contain event types and internal user IDs, never codes, secrets, passwords, or email addresses.

Favorites are an idempotent system-backed Product List and cannot be deleted through normal list operations. Favorite, list, and review actions return one JSON envelope and use the existing CSRF header. Reviews are limited to one per customer/product and can be edited or deleted only by their owner. The server derives review eligibility from an order item whose order is `shipped` or `completed`; it never accepts a “verified” flag from the browser. Templates escape review content before rendering.

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
