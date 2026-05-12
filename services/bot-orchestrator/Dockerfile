FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY services/bot-orchestrator/ ./services/bot-orchestrator/
RUN go build -o bot-orchestrator ./services/bot-orchestrator/

FROM alpine:3.23
WORKDIR /app
COPY --from=builder /app/bot-orchestrator .
CMD ["./bot-orchestrator"]