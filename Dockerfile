# Stage 1: Build
FROM golang:1.25 AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build mock server binary only
RUN go build -o /build/bin/mock-server ./cmd/mock

# Stage 2: Runtime
FROM ubuntu:24.04

# Install curl for debugging
RUN apt-get update && \
    apt-get install -y curl ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy compiled mock server binary
COPY --from=builder /build/bin/mock-server /app/mock-server

# Set mock-server as default entrypoint
ENTRYPOINT ["/app/mock-server"]

# Default command (can be overridden)
CMD []
