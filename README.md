# Ecommerce Gin

A small e-commerce backend written in Go with Gin, GORM, MySQL, and HTML templates.

## Local setup

1. Copy the template environment file:

```bash
cp .env.example .env
```

2. Fill in the local values in `.env` before starting services.
3. Start the database and application:

```bash
docker compose up --build -d
```

4. Optionally seed development users:

```bash
docker compose run --rm app /app/seed
```

The app is served on `http://localhost:8080` by default.

## Required environment variables

Use values appropriate for your environment. Do not commit real production secrets.

```env
APP_ENV=development
APP_PORT=8080
GIN_MODE=debug

MYSQL_HOST=127.0.0.1
MYSQL_PORT=3307
MYSQL_DATABASE=ecommerce
MYSQL_USER=ecommerce_app
MYSQL_PASSWORD=change_me_local

SESSION_SECRET=replace-with-a-random-secret
SESSION_SECURE=false
```

For Docker-based app containers, the application uses:

```env
MYSQL_HOST=mysql
MYSQL_PORT=3306
```

## Database and seed

The project uses MySQL with GORM. The seed command creates development/test accounts for admin, employee, and customer roles using bcrypt. It is meant for local development and test environments.

```bash
go run ./cmd/seed
```

## Security notes

- `.env` files are ignored by Git.
- `.env.example` contains placeholders only.
- Do not store production credentials in source control, README files, or Docker config.
- Do not log database passwords or session secrets.

## Development commands

```bash
go fmt ./...
go vet ./...
go test ./...
```

## Docker

```bash
docker compose up --build -d
```

The Docker stack includes the MySQL service and application service. Health checks help ensure the database is ready before the app starts accepting traffic.
