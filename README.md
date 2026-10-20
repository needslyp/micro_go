# Microservices Project

A learning project built with Go and PostgreSQL.

The project contains three services:

- **Auth** — user registration, bcrypt password verification, and JWT login.
- **Blog** — placeholder for the articles API.
- **Notification** — placeholder for registration notifications.

gRPC and Kafka integration are not implemented yet.

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

Set `AUTH_JWT_SECRET` in `.env` to a strong random secret used to sign JWTs.

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

## Log in

```bash
curl -i http://localhost:8081/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"password123"}'
```

A successful request returns `200 OK`:

```json
{"access_token":"<JWT>","token_type":"Bearer","expires_in":900}
```

Set `AUTH_JWT_TTL` to a positive Go duration in whole seconds, such as `30m`
or `1h` (default: `15m`). Invalid values prevent Auth from starting.

The HS256 token uses this lifetime and includes the user ID in `sub`,
issuer `auth-service` in `iss`, audience `blog-service` in `aud`, and `iat`/`exp` timestamps.
Use it as `Authorization: Bearer <JWT>` when calling protected endpoints.
Auth validates token signatures, HS256, expiration, issuer, audience, and a
positive integer user ID in `sub`. Future `iat` and `nbf` claims are rejected.
Protected endpoints and token validation over gRPC are not implemented yet.
Login compares the password against the stored bcrypt hash without applying
registration password length rules again.

Responses: `400` for malformed JSON, `401` for an unknown email or incorrect
password (the same error message), and `500` for internal errors.

## Checks

```bash
go test ./...
go vet ./...
```

PostgreSQL integration tests are skipped unless `AUTH_TEST_DATABASE_URL` is set.
Use a dedicated test database with permission to create schemas. Each run applies
the Auth migration in an isolated schema and removes that schema afterwards:

```bash
AUTH_TEST_DATABASE_URL='postgres://auth_test:auth_test_password@localhost:15432/auth_test?sslmode=disable' \
  go test ./internal/auth -run Integration -count=1 -v
```

The integration test covers registration, email normalization, stored bcrypt
hashes, duplicate email rejection, login JWTs, and invalid credentials.

## Stop

Stop Auth with `Ctrl+C`, then stop PostgreSQL:

```bash
docker compose down
```

Database data remains in the Docker volume.
