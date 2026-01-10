#### Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o migakustat .

#### Runtime stage
FROM alpine:latest

RUN apk add --no-cache \
    chromium \
    chromium-chromedriver \
    ca-certificates \
    tzdata \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 app && \
    adduser -D -u 1000 -G app app

WORKDIR /app

COPY --from=builder /app/migakustat .
COPY --from=builder /app/example ./example

ARG VERSION
LABEL version=$VERSION

ENV CHROME_BIN=/usr/bin/chromium-browser \
    CHROME_PATH=/usr/lib/chromium/ \
    CHROMIUM_FLAGS="--disable-software-rasterizer --disable-dev-shm-usage"

ARG MIGAKU_EMAIL
ARG MIGAKU_PASSWORD
ARG MIGAKU_HEADLESS=true

ENV MIGAKU_EMAIL=${MIGAKU_EMAIL} \
    MIGAKU_PASSWORD=${MIGAKU_PASSWORD} \
    MIGAKU_HEADLESS=${MIGAKU_HEADLESS}

RUN chown -R app:app /app

USER app

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/status || exit 1

CMD ["./migakustat"]
