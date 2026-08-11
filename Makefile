.PHONY: up down migrate run test build dashboard

up:        ## start Postgres via docker compose
	docker compose up -d

down:      ## stop Postgres
	docker compose down

migrate:   ## apply schema to $$DATABASE_URL (or the local default)
	psql "$${DATABASE_URL:-postgres://postgres:postgres@localhost:5432/orchestrator?sslmode=disable}" -f migrations/001_init.sql

run:       ## run the API + scheduler locally
	go run ./cmd/server

test:      ## run unit tests
	go test ./... -v

build:     ## compile a binary to ./bin/orchestrator
	go build -o bin/orchestrator ./cmd/server

dashboard: ## serve the Vue dashboard on :5173 (static file, no build step)
	cd web && python3 -m http.server 5173
