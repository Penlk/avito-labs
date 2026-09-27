build:
	go build -o bin/trip-service ./cmd/trip-service

run: build
	set -a; \
	. ./.env.example && \
	. ./.env && \
	exec ./bin/trip-service

generate:
	go tool oapi-codegen -generate types,chi-server -package api \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-o api/api.gen.go contracts/openapi/trip-service.openapi.yaml

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
	go test -race ./...

lint:
	go vet ./...

docker-build:
	docker build -f deploy/Dockerfile -t trip-service:lab1 .

docker-run:
	docker run --rm --name trip-service --network host --stop-timeout 15 \
		--env-file .env.example --env-file .env trip-service:lab1

docker-stop:
	docker stop trip-service
