FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY services/bot-runner/ ./services/bot-runner/
RUN go build -o bot-runner ./services/bot-runner/

FROM alpine:3.23
WORKDIR /app
COPY --from=builder /app/bot-runner .
CMD ["./bot-runner"]