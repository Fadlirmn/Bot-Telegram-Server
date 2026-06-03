# Stage 1: Build binary
FROM golang:1.24-alpine AS builder

# Use HTTP instead of HTTPS for Alpine repositories to prevent connection hangs during build
RUN sed -i 's/https/http/g' /etc/apk/repositories

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

# Copy go.mod and go.sum to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY main.go ./

# Build statically linked binary with stripped debug info (-s -w) to reduce binary size
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o monitor-bot .

# Stage 2: Minimal runtime image
FROM alpine:3.19

# Use HTTP instead of HTTPS for Alpine repositories to prevent connection hangs during build
RUN sed -i 's/https/http/g' /etc/apk/repositories

# Install ca-certificates (required for HTTPS connections to Telegram API), tzdata (for timezone support), and docker-cli
RUN apk add --no-cache ca-certificates tzdata docker-cli

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/monitor-bot .

# Start the application
ENTRYPOINT ["./monitor-bot"]
