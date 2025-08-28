# Build stage
ARG GO_VERSION=1.24
FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /workspace

# Copy the entire project
COPY . .

# Set up the module structure
WORKDIR /workspace/testframework

# Download dependencies
RUN go mod download

# Build the test runner
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-checklinkname=0" -a -installsuffix cgo -o test-runner ./test-runner

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /workspace/testframework/test-runner .

# Make the binary executable
RUN chmod +x test-runner

# Set environment variables for the Go runtime
ENV GOMEMLIMIT=512MiB
ENV GOGC=100

# Run the test runner
CMD ["./test-runner"]
