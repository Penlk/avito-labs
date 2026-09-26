generate:
	go tool oapi-codegen -generate types,chi-server -package api -o api/api.gen.go contracts/openapi/trip-service.openapi.yaml

