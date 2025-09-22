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

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./src

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS calls
RUN apk --no-cache add ca-certificates

# Create app directory
WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Copy migration files
COPY --from=builder /app/src/migrations/ ./migrations/

# Copy environment file
COPY --from=builder /app/.env ./

# Create storage directory
RUN mkdir -p ./storage

# Expose port
EXPOSE 8080

# Command to run
CMD ["./main"]