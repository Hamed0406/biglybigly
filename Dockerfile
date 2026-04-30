# Build UI
FROM node:22-alpine AS ui-builder
WORKDIR /build
COPY ui/package*.json ./
RUN npm ci
COPY ui .
RUN npm run build

# Build Go binary
FROM golang:1.22-alpine AS go-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-builder /build/dist ./internal/core/api/static
RUN CGO_ENABLED=0 GOOS=linux go build -o biglybigly ./cmd/biglybigly

# Runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
RUN addgroup -S bigly && adduser -S -G bigly bigly
COPY --from=go-builder /build/biglybigly /usr/local/bin/
COPY <<'EOF' /entrypoint.sh
#!/bin/sh
mkdir -p /data
chown -R bigly:bigly /data
exec su-exec bigly biglybigly "$@"
EOF
RUN apk add --no-cache su-exec && chmod +x /entrypoint.sh
EXPOSE 8082
ENV BIGLYBIGLY_HTTP_ADDR=:8082
ENV BIGLYBIGLY_DB_PATH=/data/biglybigly.db
ENTRYPOINT ["/entrypoint.sh"]
