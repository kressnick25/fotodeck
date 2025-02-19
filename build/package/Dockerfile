# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY . /app

RUN CGO_ENABLED=0 GOOS=linux go build -o album

# Image build stage
FROM alpine:latest

WORKDIR /app

# Copy from builder stage
COPY --from=builder /app/album .
COPY --from=builder /app/config.toml .
COPY --from=builder /app/web ./web

RUN mkdir /photos

RUN adduser -D album
USER album

EXPOSE 8080

ENTRYPOINT ["/app/album"]

CMD ["config.toml"]
