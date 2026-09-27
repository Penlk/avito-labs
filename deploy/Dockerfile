FROM golang:1.27.0-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY api ./api
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o /out/trip-service ./cmd/trip-service

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/trip-service /trip-service

USER 65532:65532
ENTRYPOINT ["/trip-service"]
