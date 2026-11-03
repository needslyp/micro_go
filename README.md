# Microservices Project

Проект на Go с тремя сервисами:

- **Auth** — регистрация, вход, JWT и проверка токенов через gRPC.
- **Blog** — API статей, в разработке.
- **Notification** — уведомления о регистрации через Kafka, в разработке.

PostgreSQL использует общую базу `microservices` с отдельными схемами сервисов.
Сейчас через Docker запускается PostgreSQL, а Auth — локально.

## Запуск

Требуются Go 1.26+, Docker Compose и CLI `golang-migrate` с поддержкой PostgreSQL.
Команды выполняются из корня проекта в Bash.

1. Создайте `.env`, если его ещё нет:

   ```bash
   cp .env.example .env
   ```

   Укажите в `.env` случайный секрет в `AUTH_JWT_SECRET`.

2. Загрузите зависимости и запустите PostgreSQL:

   ```bash
   go mod download
   docker compose up -d --wait postgres
   ```

3. Загрузите настройки и примените миграции Auth:

   ```bash
   set -a
   source .env
   set +a
   migrate -path migrations/auth -database "$AUTH_DATABASE_URL" up
   ```

   Сообщение `no change` означает, что миграции уже применены.

4. Запустите Auth:

   ```bash
   go run ./cmd/auth
   ```

   HTTP: `localhost:8081`, gRPC: `localhost:9091`.

Для остановки Auth нажмите `Ctrl+C`, затем остановите PostgreSQL:

```bash
docker compose down
```

Данные сохраняются в Docker volume.
