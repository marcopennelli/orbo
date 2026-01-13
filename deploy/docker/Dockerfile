# =============================================================================
# Orbo - Multi-stage Docker Build
# =============================================================================
# Builds the main orbo application (Go backend + React frontend)
# Usage: docker build -f deploy/docker/Dockerfile -t orbo:latest .
# =============================================================================

# -----------------------------------------------------------------------------
# Stage 1: Build Frontend
# -----------------------------------------------------------------------------
FROM node:20-alpine AS frontend-builder

WORKDIR /app/web/frontend

# Copy package files first for better caching
COPY web/frontend/package*.json ./
RUN npm ci --only=production=false

# Copy frontend source and build
COPY web/frontend/ ./
RUN npm run build

# -----------------------------------------------------------------------------
# Stage 2: Build Go Backend
# -----------------------------------------------------------------------------
FROM golang:1.23-alpine AS backend-builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY api/ ./api/
COPY *.go ./

# Copy built frontend from previous stage
COPY --from=frontend-builder /app/web/frontend/dist ./web/frontend/dist

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /orbo ./cmd/orbo

# -----------------------------------------------------------------------------
# Stage 3: Final Runtime Image
# -----------------------------------------------------------------------------
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 orbo && \
    adduser -u 1000 -G orbo -s /bin/sh -D orbo

WORKDIR /app

# Copy binary from builder
COPY --from=backend-builder /orbo /app/orbo

# Copy frontend assets
COPY --from=frontend-builder /app/web/frontend/dist /app/web/frontend/dist

# Create directories for runtime data
RUN mkdir -p /app/frames /app/data && \
    chown -R orbo:orbo /app

# Switch to non-root user
USER orbo

# Expose HTTP port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q --spider http://localhost:8080/healthz || exit 1

# Default environment variables
ENV ORBO_HOST=0.0.0.0 \
    ORBO_PORT=8080 \
    ORBO_DB_PATH=/app/data/orbo.db \
    ORBO_FRAMES_PATH=/app/frames

# Run the application
ENTRYPOINT ["/app/orbo"]
