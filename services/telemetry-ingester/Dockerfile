FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY services/telemetry-ingester/ ./services/telemetry-ingester/
RUN go build -o telemetry-ingester ./services/telemetry-ingester/

FROM alpine:3.23
WORKDIR /app
COPY --from=builder /app/telemetry-ingester .
CMD ["./telemetry-ingester"]
