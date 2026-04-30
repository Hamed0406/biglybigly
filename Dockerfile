# Build UI
FROM node:22-alpine AS ui-builder
WORKDIR /build
COPY ui/package*.json ./
RUN npm ci
COPY ui .
RUN npm run build

# Build Go binary (multi-arch aware)
FROM golang:1.22-alpine AS go-builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG TARGETVARIANT
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-builder /build/dist ./internal/core/api/static
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags "-s -w" -o biglybigly ./cmd/biglybigly

# Runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates su-exec
RUN addgroup -S bigly && adduser -S -G bigly bigly
COPY --from=go-builder /build/biglybigly /usr/local/bin/
COPY <<'EOF' /entrypoint.sh
#!/bin/sh
mkdir -p /data
chown -R bigly:bigly /data
exec su-exec bigly biglybigly "$@"
EOF
RUN chmod +x /entrypoint.sh
EXPOSE 8082
ENV BIGLYBIGLY_HTTP_ADDR=:8082
ENV BIGLYBIGLY_DB_PATH=/data/biglybigly.db
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8082/api/modules || exit 1
ENTRYPOINT ["/entrypoint.sh"]
