FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY services/submission-api/ ./services/submission-api/
RUN go build -o submission-api ./services/submission-api/

FROM alpine:3.23
WORKDIR /app
COPY --from=builder /app/submission-api .
EXPOSE 8080
CMD ["./submission-api"]
