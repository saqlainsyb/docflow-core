.PHONY: dev build run migrate-up migrate-down docker-up docker-down tidy

# start dev server with hot reload
dev:
	air

# build the binary
build:
	go build -o bin/docflow cmd/api/main.go

# run without hot reload
run:
	go run cmd/api/main.go

# apply all migrations
migrate-up:
	go run cmd/migrate/main.go up

# rollback last migration
migrate-down:
	go run cmd/migrate/main.go down

# start docker containers
docker-up:
	docker compose up -d

# stop docker containers
docker-down:
	docker compose down

# stop docker and wipe volumes (fresh start)
docker-reset:
	docker compose down -v

# tidy dependencies
tidy:
	go mod tidy