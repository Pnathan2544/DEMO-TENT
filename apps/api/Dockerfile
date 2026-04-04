# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Download dependencies first (cached layer if go.mod/go.sum unchanged).
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -ldflags="-w -s" -o /api ./cmd/api/...

# ── Run stage ─────────────────────────────────────────────────────────────────
FROM alpine:3.19

# ca-certificates: needed for HTTPS calls to Stripe API.
# tzdata: needed for correct time zone handling in logs.
RUN apk --no-cache add ca-certificates tzdata

# Run as non-root.
RUN addgroup -g 1001 -S app && adduser -u 1001 -S app -G app

WORKDIR /app
COPY --from=builder /api ./api

USER app
EXPOSE 8080
CMD ["./api"]
