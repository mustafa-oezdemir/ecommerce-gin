# Ecommerce Gin

A secure, server-rendered e-commerce demo built with Go, Gin, GORM, and MySQL. It includes customer shopping flows, operational tooling for employees, and an admin back office—ready to run as a local Docker stack.

## Highlights

- **Customer experience** — browse products, manage a cart, complete checkout, and view orders.
- **Operations** — manage products, inventory, and order status as an employee.
- **Administration** — review dashboards, users, categories, and orders.
- **Security by default** — signed sessions, RBAC, ownership checks, CSRF protection, secure headers, validated requests, and rate-limited sign-in.
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
