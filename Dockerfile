# Stage 1: Build binary
FROM golang:1.21-alpine AS builder

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

# Install ca-certificates (required for HTTPS connections to Telegram API) and tzdata (for timezone support)
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/monitor-bot .

# Start the application
ENTRYPOINT ["./monitor-bot"]
