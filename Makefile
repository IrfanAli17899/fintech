DB_URL ?= postgres://fintech:fintech@localhost:5432/fintech?sslmode=disable

run:
	DATABASE_URL="$(DB_URL)" go run ./cmd/server

up:
	docker compose up -d

db-reset:
	docker compose exec postgres psql -U fintech -d fintech -c \
	  "DROP SCHEMA IF EXISTS public CASCADE; DROP SCHEMA IF EXISTS ledger CASCADE; DROP SCHEMA IF EXISTS rails CASCADE; CREATE SCHEMA public;"

db-shell:
	docker compose exec postgres psql -U fintech -d fintech

TEST_DB_URL ?= postgres://fintech:fintech@localhost:5432/fintech_test?sslmode=disable

# Tests never touch the dev database: they leave orphans and divergences by
# design, and the ledger is append-only, so they could never be cleaned up.
test-db:
	docker compose exec -T postgres psql -U fintech -d fintech -tAc \
	  "SELECT 1 FROM pg_database WHERE datname = 'fintech_test'" | grep -q 1 || \
	docker compose exec -T postgres psql -U fintech -d fintech -c "CREATE DATABASE fintech_test"

test-db-reset:
	docker compose exec -T postgres psql -U fintech -d fintech -c "DROP DATABASE IF EXISTS fintech_test"
	$(MAKE) test-db

# -p 1: tests share a database; parallel packages race on rail outcomes.
# Do not remove until each package gets its own database (testcontainers).
test: test-db
	TEST_DATABASE_URL="$(TEST_DB_URL)" go test -count=1 -race -p 1 ./...