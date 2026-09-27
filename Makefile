generate:
	go tool oapi-codegen -generate types,chi-server -package api \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-o api/api.gen.go contracts/openapi/trip-service.openapi.yaml
