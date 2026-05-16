FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY services/submission-api/ ./services/submission-api/
RUN go build -o submission-api ./services/submission-api/

FROM alpine:3.23
WORKDIR /app
COPY --from=builder /app/submission-api .
USER 65534
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
EXPOSE 8080
CMD ["./submission-api"]
