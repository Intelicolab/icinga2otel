
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY main.go ./
COPY internal ./internal
COPY config/config.toml.example config/config.toml.example

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /icinga2otel



FROM alpine:3

COPY --from=builder /icinga2otel /icinga2otel
COPY --from=builder app/config /config

ENTRYPOINT ["/icinga2otel"]
CMD ["/icinga2otel"]


