#### Build stage
FROM golang:1.26-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY . .
RUN if [ -f examples/discord-nickname/main.go ]; then \
      CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o discord-nickname ./examples/discord-nickname; \
    else \
      [ -f go.mod ] || go mod init discord-nickname; \
      CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o discord-nickname .; \
    fi

#### Runtime stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 app && \
    adduser -D -u 1000 -G app app

WORKDIR /app

COPY --from=builder /app/discord-nickname .

RUN chown -R app:app /app

USER app

CMD ["./discord-nickname"]
