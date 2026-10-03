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

migrate-up:
	migrate -path migrations -database "$(DB_URL)" up

migrate-down:
	migrate -path migrations -database "$(DB_URL)" down 1

test:
	go test -count=1 -race ./...