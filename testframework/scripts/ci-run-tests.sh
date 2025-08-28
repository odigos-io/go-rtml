#!/bin/bash

# Go RTML Test Framework CI Runner
# This script is optimized for CI environments with enhanced error reporting

set -e

# Colors for output (simplified for CI)
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

# Function to print CI-friendly output
print_ci_status() {
    echo "##[section]$1"
}

print_ci_error() {
    echo "##[error]$1"
}

print_ci_warning() {
    echo "##[warning]$1"
}

# Check prerequisites
check_prerequisites() {
    print_ci_status "Checking prerequisites..."
    
    # Check if Docker is running
    if ! docker info >/dev/null 2>&1; then
        print_ci_error "Docker is not running"
        exit 1
    fi
    
    # Check if Go is installed
    if ! command -v go >/dev/null 2>&1; then
        print_ci_error "Go is not installed"
        exit 1
    fi
    
    # Check Go version
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    print_status "Running tests with Go version: $GO_VERSION"
    
    print_success "Prerequisites check passed"
}

# Install dependencies
install_deps() {
    print_ci_status "Installing dependencies..."
    go mod download
    go mod tidy
    print_success "Dependencies installed"
}

# Build binaries
build_binaries() {
    print_ci_status "Building binaries..."
    
    # Create bin directory
    mkdir -p bin
    
    # Build test framework
    print_status "Building test framework..."
    if ! go build -o bin/test-framework ./test-framework; then
        print_ci_error "Failed to build test framework"
        exit 1
    fi
    
    # Build test runner
    print_status "Building test runner..."
    if ! go build -ldflags="-checklinkname=0" -o bin/test-runner ./test-runner; then
        print_ci_error "Failed to build test runner"
        exit 1
    fi
    
    print_success "All binaries built successfully"
}

# Build Docker image
build_docker_image() {
    print_ci_status "Building Docker image..."
    
    if ! docker build -t go-rtml-test:latest .; then
        print_ci_error "Failed to build Docker image"
        exit 1
    fi
    
    print_success "Docker image built successfully"
}

# Run tests
run_tests() {
    print_ci_status "Running test suite..."
    
    # Create results directory
    mkdir -p test-results
    
    # Run the test framework
    print_status "Executing test framework..."
    if ! ./bin/test-framework; then
        print_ci_warning "Test framework execution had issues"
    fi
    
    print_success "Test suite completed"
}

# Generate test report
generate_report() {
    print_ci_status "Generating test report..."
    
    if [ ! -f "test-results/test-report.json" ]; then
        print_ci_error "No test results found - test execution failed completely"
        return 1
    fi
    
    # Count test results
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
    
    echo ""
    echo "Test Summary for Go $GO_VERSION:"
    echo "Total tests: $TOTAL"
    echo "Failed tests: $FAILED"
    echo "Timeout tests: $TIMEOUT"
    
    # Show detailed failure information
    if [ "$FAILED" -gt 0 ] || [ "$TIMEOUT" -gt 0 ]; then
        echo ""
        echo "=== FAILURE DETAILS ==="
        
        if command -v jq >/dev/null 2>&1; then
            # Extract and display failure details
            jq -r '.[] | select(.status != "passed") | 
                "❌ " + .test_name + " (" + .status + ")\n" +
                "   Exit Code: " + (.exit_code | tostring) + "\n" +
                "   Error: " + (.error // "N/A") + "\n" +
                (if .failure_details.reason then "   Reason: " + .failure_details.reason + "\n" else "" end) +
                (if .failure_details.expected_value then "   Expected: " + .failure_details.expected_value + "\n" else "" end) +
                (if .failure_details.actual_value then "   Actual: " + .failure_details.actual_value + "\n" else "" end) +
                (if .failure_details.log_snippet then "   Log Snippet:\n" + (.failure_details.log_snippet | split("\n") | map("     " + .) | join("\n")) + "\n" else "" end) +
                "\n"' test-results/test-report.json
        else
            # Fallback for environments without jq
            echo "Detailed failure information available in test-results/test-report.json"
            echo "Install jq for better error reporting"
        fi
        
        # Determine if tests should be considered failed
        if [ "$FAILED" -gt 0 ] || [ "$TIMEOUT" -gt 0 ]; then
            print_ci_error "Some tests failed or timed out with Go $GO_VERSION"
            return 1
        fi
    fi
    
    if [ "$FAILED" -eq 0 ] && [ "$TIMEOUT" -eq 0 ]; then
        print_success "All tests passed with Go $GO_VERSION"
        return 0
    fi
    
    return 1
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
    echo "    Go RTML Test Framework CI Runner"
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
            generate_report
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
            echo ""
            echo "This script is optimized for CI environments with enhanced error reporting."
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
