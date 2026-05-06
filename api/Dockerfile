# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

# Install git and ca-certs (needed for go get over HTTPS)
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Cache module downloads separately from source code
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a statically linked binary with security hardening
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -trimpath \
      -ldflags="-s -w -extldflags '-static'" \
      -o openseedr-api \
      .

# ── Stage 2: minimal runtime ──────────────────────────────────────────────────
FROM scratch

# Pull in TLS roots and timezone data from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Non-root user (numeric IDs work in scratch)
USER 10001:10001

COPY --from=builder /build/openseedr-api /openseedr-api

EXPOSE 8080

ENTRYPOINT ["/openseedr-api"]
