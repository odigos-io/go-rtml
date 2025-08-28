.PHONY: build build-test-runner build-test-framework docker-build docker-run-tests run-test clean help deps

# Default target
help:
	@echo "Available targets:"
	@echo "  build-test-runner     - Build the test runner binary"
	@echo "  build-test-framework  - Build the test framework binary"
	@echo "  docker-build          - Build the Docker image for tests"
	@echo "  docker-run-tests      - Run the complete test suite"
	@echo "  run-test              - Run test locally and show results"
	@echo "  clean                 - Clean build artifacts"
	@echo "  deps                  - Install dependencies"
	@echo "  help                  - Show this help message"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Build the test runner binary
build-test-runner:
	@echo "Building test runner..."
	go build -ldflags="-checklinkname=0" -o bin/test-runner ./test-runner

# Build the test framework binary
build-test-framework:
	@echo "Building test framework..."
	go build -o bin/test-framework ./test-framework

# Build both binaries
build: build-test-runner build-test-framework

# Build the Docker image
docker-build:
	@echo "Building Docker image..."
	./build-docker.sh

# Run the test suite
docker-run-tests: docker-build
	@echo "Running test suite..."
	@mkdir -p test-results
	./bin/test-framework

# Run test locally and show results
run-test: build-test-runner
	@echo "=========================================="
	@echo "    Running Sanity Check Test Locally"
	@echo "=========================================="
	@echo ""
	./bin/test-runner
	@echo ""
	@echo "=========================================="
	@echo "    Test Completed Successfully!"
	@echo "=========================================="

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -rf test-results/
	docker rmi go-rtml-test:latest 2>/dev/null || true
