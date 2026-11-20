.PHONY: build run generate migrate migrate-down migrate-status test test-integration lint docker-build docker-run docker-stop

build:
	go build -o bin/trip-service ./cmd/service

run: build
	set -a; \
	. ./.env.example && \
	. ./.env && \
	exec ./bin/trip-service

generate:
	mkdir -p internal/generated
	go tool oapi-codegen -config oapi-codegen.yaml \
		contracts/openapi/trip-service.openapi.yaml
	go generate ./internal/...

migrate:
	. ./.env && \
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$DATABASE_URL" \
	go tool goose -env=none -dir migrations up

migrate-down:
	. ./.env && \
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$DATABASE_URL" \
	go tool goose -env=none -dir migrations down

migrate-status:
	. ./.env && \
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$DATABASE_URL" \
	go tool goose -env=none -dir migrations status

test:
	go test -race -count=2 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

test-integration:
	set -a; . ./.env.example && . ./.env && \
	go test -race -count=2 -tags=integration ./internal/repository/...

lint:
	golangci-lint run ./...

docker-build:
	docker build -f deploy/Dockerfile -t trip-service:lab1 .

docker-run:
	docker run --rm --name trip-service --network host --stop-timeout 15 \
		--env-file .env.example --env-file .env trip-service:lab1

docker-stop:
	docker stop trip-service
