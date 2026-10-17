# Microservices Project

A learning project built with Go and PostgreSQL.

The project contains three services:

- **Auth** — user registration with bcrypt password hashing.
- **Blog** — placeholder for the articles API.
- **Notification** — placeholder for registration notifications.

Login, JWT, gRPC, and Kafka integration are not implemented yet.

## Requirements

- Go 1.26 or newer
- Docker with Docker Compose
- golang-migrate CLI with PostgreSQL support

Run all commands from the project root.

## Local setup

Create the configuration file if it does not exist:

```bash
cp .env.example .env
```

Set `AUTH_JWT_SECRET` in `.env`. It is required by the current
configuration loader, although JWT is not implemented yet.

Download Go dependencies:

```bash
go mod tidy
```

Start PostgreSQL:

```bash
docker compose up -d --wait postgres
```

Load environment variables into your shell and apply migrations:

```bash
set -a
source .env
set +a

migrate -path migrations/auth -database "$AUTH_DATABASE_URL" up
```

The environment-loading commands above are for Bash.
`no change` means all migrations have already been applied.

Start Auth:

```bash
go run ./cmd/auth
```

By default, Auth listens on `http://localhost:8081`.
Only PostgreSQL runs in Docker; Auth runs on the host.

## Register a user

Run in another terminal:

```bash
curl -i http://localhost:8081/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"password123"}'
```

A successful request returns `201 Created` with the user's
`id`, `email`, and `created_at`. Passwords and hashes are not returned.

Other responses:

- `400` — invalid input
- `409` — email already registered
- `500` — internal error

Passwords must contain at least 8 Unicode code points
and must not exceed 72 bytes.

## Checks

```bash
go test ./...
go vet ./...
```

## Stop

Stop Auth with `Ctrl+C`, then stop PostgreSQL:

```bash
docker compose down
```

Database data remains in the Docker volume.
