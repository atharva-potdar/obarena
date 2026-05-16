FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY services/leaderboard-ws/ ./services/leaderboard-ws/
RUN go build -o leaderboard-ws ./services/leaderboard-ws/

FROM alpine:3.23
WORKDIR /app
COPY --from=builder /app/leaderboard-ws .
USER 65534
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:8090/healthz || exit 1
CMD ["./leaderboard-ws"]
