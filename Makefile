.PHONY: run build test migrate-up migrate-down docker-up docker-down deps lint

# Development
run:
	go run main.go

build:
	go build -o bin/server main.go

test:
	go test -v ./...

# Database
migrate-up:
	@echo "Running migrations..."
	@for f in migrations/*.up.sql; do \
		echo "Applying $$f"; \
		psql "$(DATABASE_URL)" -f "$$f"; \
	done

migrate-down:
	@echo "Rolling back migrations..."
	@for f in $$(ls -r migrations/*.down.sql 2>/dev/null); do \
		echo "Rolling back $$f"; \
		psql "$(DATABASE_URL)" -f "$$f"; \
	done

# Docker
docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

# Dependencies
deps:
	go mod download
	go mod tidy

# Linting
lint:
	golangci-lint run

# All-in-one development setup
dev: docker-up
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 3
	@$(MAKE) migrate-up
	@$(MAKE) run
