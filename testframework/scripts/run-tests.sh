#!/bin/bash

# Go RTML Test Framework Runner
# This script builds and runs the complete test suite

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    print_status "Checking prerequisites..."
    
    # Check if Docker is running
    if ! docker info >/dev/null 2>&1; then
        print_error "Docker is not running. Please start Docker and try again."
        exit 1
    fi
    
    # Check if Go is installed
    if ! command -v go >/dev/null 2>&1; then
        print_error "Go is not installed. Please install Go 1.24 or later."
        exit 1
    fi
    
    # Check Go version
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    REQUIRED_VERSION="1.24"
    
    if [ "$(printf '%s\n' "$REQUIRED_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$REQUIRED_VERSION" ]; then
        print_error "Go version $GO_VERSION is too old. Please install Go $REQUIRED_VERSION or later."
        exit 1
    fi
    
    print_success "Prerequisites check passed"
}

# Install dependencies
install_deps() {
    print_status "Installing dependencies..."
    go mod download
    go mod tidy
    print_success "Dependencies installed"
}

# Build binaries
build_binaries() {
    print_status "Building binaries..."
    
    # Create bin directory
    mkdir -p bin
    
    # Build test framework
    print_status "Building test framework..."
    go build -o bin/test-framework ./cmd/test-framework
    
    # Build test runner
    print_status "Building test runner..."
    go build -ldflags="-checklinkname=0" -o bin/test-runner ./cmd/test-runner
    
    print_success "Binaries built successfully"
}

# Build Docker image
build_docker_image() {
    print_status "Building Docker image..."
    docker build -t go-rtml-test:latest .
    print_success "Docker image built successfully"
}

# Run tests
run_tests() {
    print_status "Running test suite..."
    
    # Create results directory
    mkdir -p test-results
    
    # Run the test framework
    ./bin/test-framework
    
    print_success "Test suite completed"
}

# Show results
show_results() {
    print_status "Test results:"
    
    if [ -f "test-results/test-report.json" ]; then
        echo ""
        echo "=== Test Results Summary ==="
        
        # Count test results using jq if available, otherwise use grep
        if command -v jq >/dev/null 2>&1; then
            TOTAL=$(jq length test-results/test-report.json)
            PASSED=$(jq '[.[] | select(.status == "passed")] | length' test-results/test-report.json)
            FAILED=$(jq '[.[] | select(.status == "failed")] | length' test-results/test-report.json)
            TIMEOUT=$(jq '[.[] | select(.status == "timeout")] | length' test-results/test-report.json)
        else
            TOTAL=$(grep -c '"test_name"' test-results/test-report.json)
            PASSED=$(grep -c '"status": "passed"' test-results/test-report.json)
            FAILED=$(grep -c '"status": "failed"' test-results/test-report.json)
            TIMEOUT=$(grep -c '"status": "timeout"' test-results/test-report.json)
        fi
        
        echo "Total Tests: $TOTAL"
        echo "Passed: $PASSED"
        echo "Failed: $FAILED"
        echo "Timeout: $TIMEOUT"
        echo ""
        echo "Detailed results saved to: test-results/test-report.json"
    else
        print_warning "No test results found"
    fi
}

# Cleanup function
cleanup() {
    print_status "Cleaning up..."
    
    # Remove binaries
    rm -rf bin/
    
    # Remove test results
    rm -rf test-results/
    
    # Remove Docker image
    docker rmi go-rtml-test:latest 2>/dev/null || true
    
    print_success "Cleanup completed"
}

# Main execution
main() {
    echo "=========================================="
    echo "    Go RTML Test Framework Runner"
    echo "=========================================="
    echo ""
    
    # Parse command line arguments
    case "${1:-run}" in
        "run")
            check_prerequisites
            install_deps
            build_binaries
            build_docker_image
            run_tests
            show_results
            ;;
        "build")
            check_prerequisites
            install_deps
            build_binaries
            build_docker_image
            print_success "Build completed"
            ;;
        "clean")
            cleanup
            ;;
        "help"|"-h"|"--help")
            echo "Usage: $0 [command]"
            echo ""
            echo "Commands:"
            echo "  run     - Run the complete test suite (default)"
            echo "  build   - Build binaries and Docker image only"
            echo "  clean   - Clean up build artifacts"
            echo "  help    - Show this help message"
            ;;
        *)
            print_error "Unknown command: $1"
            echo "Use '$0 help' for usage information"
            exit 1
            ;;
    esac
}

# Run main function
main "$@"
