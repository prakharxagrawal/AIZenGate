# =============================================================================
# ZenGate AI — Multi-stage Dockerfile
# Stage 1: Build the Go binary
# Stage 2: Minimal runtime image (distroless)
# =============================================================================

# --- Build Stage ---
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /zengate ./cmd/gateway

# --- Runtime Stage ---
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /zengate /zengate

# Expose gateway port
EXPOSE 8080

# Run as non-root user
USER nonroot:nonroot

ENTRYPOINT ["/zengate"]
