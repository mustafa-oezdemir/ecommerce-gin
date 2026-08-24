# Ecommerce Gin

Secure demo e-commerce application built with Go, Gin, GORM, MySQL and server-rendered HTML.

## Features

- Customer product browsing, cart, atomic checkout, account and own-order views.
- Employee product, inventory and order processing screens.
- Admin dashboards, user creation and category management.
- Signed cookie sessions, RBAC, ownership-scoped data access, CSRF protection, request validation and rate-limited login.
- Integer-cent money values and immutable order-item snapshots.
- Versioned SQL migration, idempotent development seed and Docker/MailHog development stack.

## Roles

| Role | Capabilities |
| --- | --- |
| Customer | Browse products, manage own cart, checkout, update own profile/password, see own orders |
| Employee | Operational dashboard, products, inventory and order status transitions |
| Admin | Employee capabilities plus dashboards, users and category management |

## Setup

1. Copy `.env.example` to `.env`.
2. Replace all password and secret placeholders. `SESSION_SECRET` must be at least 32 characters. `CSRF_SECRET` must be base64-encoded 32 random bytes.
3. Start the local stack:

```bash
docker compose up --build -d
docker compose run --rm app /app/seed
```

The application is at `http://localhost:8080`; MailHog is at `http://localhost:8025`; Prometheus is at `http://localhost:9090`; Grafana is at `http://localhost:3000`.

Seed users exist only for development/test:

| Role | Email | Password |
| --- | --- | --- |
| Admin | `admin@example.com` | `AdminPass123!` |
| Employee | `employee@example.com` | `EmployeePass123!` |
| Customer | `customer@example.com` | `CustomerPass123!` |

Never use these seed accounts or their passwords in production. The seed command refuses to run when `APP_ENV=production`.

## Configuration

Required application variables are documented in `.env.example`. Docker passes only application-required values to the app container; the MySQL root password is not exposed to it. `SMTP_*` targets MailHog only and is disabled by the application in production.

Use `MYSQL_HOST=127.0.0.1` and `MYSQL_PORT=3307` for a host process; the Docker application receives `MYSQL_HOST=mysql` and port `3306`.

## Database

`migrations/000001_initial.sql` is applied once and tracked in `schema_migrations`. It defines indexes, foreign keys, user-email uniqueness, integer-cent money columns and cart/order ownership relationships.

## Health checks

- `GET /health/live` checks whether the process is running.
- `GET /health/ready` verifies database connectivity.

Neither endpoint exposes configuration or secrets.

## Monitoring

The application exposes Prometheus metrics only on its internal Docker port `9091`; it is scraped by Prometheus as `app:9091` and is not published by Compose. Grafana is provisioned automatically with the Prometheus datasource and an **Ecommerce Overview** dashboard. Metric labels use Gin route templates rather than raw URLs and never include customer or secret values.

## Development

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
go build ./...
docker compose config
```

CI runs formatting, vet, unit tests, race detection and builds. The security workflow runs `govulncheck` and `gosec`; Dependabot tracks Go, Docker and GitHub Actions updates.

## Production notes

Run behind an HTTPS reverse proxy and set `APP_ENV=production`, `GIN_MODE=release` and `SESSION_SECURE=true`. Supply unique secrets through a secrets manager, keep MySQL on a private network, configure backups/monitoring, and use a real production mail provider only through a separately reviewed integration. MailHog is intentionally development/test only.
