FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY services/bot-orchestrator/ ./services/bot-orchestrator/
RUN go build -o bot-orchestrator ./services/bot-orchestrator/

FROM alpine:3.23
WORKDIR /app
COPY --from=builder /app/bot-orchestrator .
USER 65534
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
CMD ["./bot-orchestrator"]
