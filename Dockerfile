# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build all binaries
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/controller ./cmd/controller
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/jobctl ./cmd/cli

# Controller image
FROM alpine:3.20 AS controller

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /bin/controller /app/controller

EXPOSE 6161

ENTRYPOINT ["/app/controller"]

# Worker image
FROM alpine:3.20 AS worker

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /bin/worker /app/worker

ENTRYPOINT ["/app/worker"]

# CLI image (optional, for running CLI in containers)
FROM alpine:3.20 AS cli

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /bin/jobctl /app/jobctl

ENTRYPOINT ["/app/jobctl"]

# Default: controller
FROM controller
