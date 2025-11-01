# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY . /app

RUN CGO_ENABLED=0 GOOS=linux go build -o fotodeck

# Image build stage
FROM alpine:latest

WORKDIR /app

# Copy from builder stage
COPY --from=builder /app/fotodeck .
COPY --from=builder /app/config.toml .
COPY --from=builder /app/web ./web

RUN mkdir /photos

RUN adduser -D fotodeck
USER fotodeck

EXPOSE 8080

ENTRYPOINT ["/app/fotodeck"]

CMD ["config.toml"]
