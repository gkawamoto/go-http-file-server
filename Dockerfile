# Build stage
FROM golang:1.25.5-alpine AS builder

# Set working directory
WORKDIR /app

# Install git and ca-certificates (needed for downloading dependencies)
RUN apk add --no-cache git ca-certificates

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o go-http-file-server .

# Final stage - use scratch for minimal image size
FROM scratch

# Copy ca-certificates from builder (needed for HTTPS requests if any)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary from builder stage
COPY --from=builder /app/go-http-file-server /go-http-file-server

# Expose port 8080 (default port for the application)
EXPOSE 8080

WORKDIR /data

# Create a volume for data storage
VOLUME ["/data"]

# Set the binary as entrypoint
ENTRYPOINT ["/go-http-file-server"]

# Default command arguments
CMD ["/go-http-file-server", "-addr", "0.0.0.0", "-port", "8080", "-dir", "/data"]