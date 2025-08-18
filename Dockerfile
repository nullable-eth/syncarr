# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install git for version detection
RUN apk add --no-cache git

# Copy source code (includes go.mod)
COPY . .
RUN go mod download

# Build arguments for version information (can be provided by CI/CD)
ARG VERSION
ARG COMMIT
ARG BUILD_DATE

# Auto-detect version information if not provided via build args
RUN if [ -z "$VERSION" ]; then \
     VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev-local"); \
     fi && \
     if [ -z "$COMMIT" ]; then \
     COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "local"); \
     fi && \
     if [ -z "$BUILD_DATE" ]; then \
     BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ"); \
     fi && \
     echo "Building with VERSION=$VERSION, COMMIT=$COMMIT, BUILD_DATE=$BUILD_DATE" && \
     CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
     -ldflags "-X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$BUILD_DATE" \
     -o syncarr ./cmd/syncarr

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests and debugging tools
RUN apk update && apk upgrade && \
     apk add --no-cache ca-certificates tzdata bash curl wget busybox-extras rsync sshpass openssh-client && \
     ln -sf /bin/bash /bin/sh && \
     echo "Bash installed successfully" && \
     which bash && bash --version

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/syncarr .

# Create a non-root user
RUN adduser -D -s /bin/bash syncarr && \
     chown syncarr:syncarr ./syncarr && \
     chmod +x ./syncarr

USER syncarr

# Run the application
CMD ["./syncarr"] 