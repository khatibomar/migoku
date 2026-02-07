#### Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o migoku .

#### Runtime stage
FROM alpine:latest

# Create non-root user
RUN addgroup -g 1000 app && \
    adduser -D -u 1000 -G app app

WORKDIR /app

COPY --from=builder /app/migoku .
COPY --from=builder /app/example ./example

ARG LOG_LEVEL=info
ARG PORT=8080
ARG API_SECRET
ARG CORS_ORIGINS
ARG CACHE_TTL

ENV LOG_LEVEL=${LOG_LEVEL} \
    PORT=${PORT} \
    API_SECRET=${API_SECRET} \
    CORS_ORIGINS=${CORS_ORIGINS} \
    CACHE_TTL=${CACHE_TTL}

RUN chown -R app:app /app

USER app

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${PORT}/api/status || exit 1

CMD ["./migoku"]
