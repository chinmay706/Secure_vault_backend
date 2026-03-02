# Build stage
FROM golang:1.22-alpine AS builder

# Allow Go to auto-install a newer toolchain if required by go.mod
ENV GOTOOLCHAIN=auto

# Set working directory
WORKDIR /app

# Install git (needed for go modules)
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy entire source code to match module structure
COPY . .

# Build the application from module root using full module path
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main securevault-backend/src

# Final stage
FROM alpine:3.19

# Install ca-certificates for HTTPS calls
RUN apk --no-cache add ca-certificates

# Create non-root user for security
RUN adduser -D -g '' appuser

# Create app directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Copy migration files
COPY --from=builder /app/src/migrations/ ./migrations/

# Create storage directory and set ownership
RUN mkdir -p ./storage && chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

# Command to run
CMD ["./main"]