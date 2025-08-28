#!/bin/bash

# Go RTML Test Framework Runner
# This script builds and runs the complete test suite

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
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

print_debug() {
    echo -e "${PURPLE}[DEBUG]${NC} $1"
}

print_test() {
    echo -e "${CYAN}[TEST]${NC} $1"
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
    print_debug "Using Go version: $GO_VERSION"
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
    if go build -o bin/test-framework ./test-framework; then
        print_success "Test framework built successfully"
    else
        print_error "Failed to build test framework"
        exit 1
    fi
    
    # Build test runner
    print_status "Building test runner..."
    if go build -ldflags="-checklinkname=0" -o bin/test-runner ./test-runner; then
        print_success "Test runner built successfully"
    else
        print_error "Failed to build test runner"
        exit 1
    fi
    
    print_success "All binaries built successfully"
}

# Build Docker image
build_docker_image() {
    print_status "Building Docker image..."
    
    # Check if Dockerfile exists
    if [ ! -f "Dockerfile" ]; then
        print_error "Dockerfile not found in current directory"
        exit 1
    fi
    
    if docker build -t go-rtml-test:latest .; then
        print_success "Docker image built successfully"
    else
        print_error "Failed to build Docker image"
        exit 1
    fi
}

# Run tests
run_tests() {
    print_status "Running test suite..."
    
    # Create results directory
    mkdir -p test-results
    
    # Run the test framework and capture output
    print_test "Executing test framework..."
    if ./bin/test-framework; then
        print_success "Test framework executed successfully"
    else
        print_error "Test framework execution failed"
        # Don't exit here, let's see what results we got
    fi
    
    print_success "Test suite completed"
}

# Show results with enhanced error reporting
show_results() {
    print_status "Analyzing test results..."
    
    if [ -f "test-results/test-report.json" ]; then
        echo ""
        echo "=========================================="
        echo "           TEST RESULTS SUMMARY"
        echo "=========================================="
        
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
        
        # Show detailed results
        if [ "$FAILED" -gt 0 ] || [ "$TIMEOUT" -gt 0 ]; then
            echo "=========================================="
            echo "           FAILURE DETAILS"
            echo "=========================================="
            
            if command -v jq >/dev/null 2>&1; then
                # Use jq to extract detailed failure information
                jq -r '.[] | select(.status != "passed") | 
                    "❌ Test: " + .test_name + "\n" +
                    "   Status: " + .status + "\n" +
                    "   Duration: " + (.duration_seconds | tostring) + " seconds\n" +
                    "   Exit Code: " + (.exit_code | tostring) + "\n" +
                    "   Error: " + (.error // "N/A") + "\n" +
                    (if .failure_details.reason then "   Reason: " + .failure_details.reason + "\n" else "" end) +
                    (if .failure_details.expected_value then "   Expected: " + .failure_details.expected_value + "\n" else "" end) +
                    (if .failure_details.actual_value then "   Actual: " + .failure_details.actual_value + "\n" else "" end) +
                    (if .failure_details.log_snippet then "   Log Snippet:\n" + (.failure_details.log_snippet | split("\n") | map("     " + .) | join("\n")) + "\n" else "" end) +
                    (if .memory_stats.peak_memory_mb > 0 then "   Peak Memory: " + (.memory_stats.peak_memory_mb | tostring) + " MB\n" else "" end) +
                    "\n"' test-results/test-report.json
            else
                # Fallback to grep-based parsing
                print_warning "jq not available, showing basic failure information"
                grep -A 20 '"status": "failed"' test-results/test-report.json || true
                grep -A 20 '"status": "timeout"' test-results/test-report.json || true
            fi
        fi
        
        # Show success details
        if [ "$PASSED" -gt 0 ]; then
            echo "=========================================="
            echo "           SUCCESS DETAILS"
            echo "=========================================="
            
            if command -v jq >/dev/null 2>&1; then
                jq -r '.[] | select(.status == "passed") | 
                    "✅ Test: " + .test_name + 
                    " (" + (.duration_seconds | tostring) + "s, Peak: " + (.memory_stats.peak_memory_mb | tostring) + " MB)\n"' test-results/test-report.json
            else
                grep -A 5 '"status": "passed"' test-results/test-report.json || true
            fi
        fi
        
        echo ""
        echo "Detailed results saved to: test-results/test-report.json"
        
        # Determine overall success/failure
        if [ "$FAILED" -eq 0 ] && [ "$TIMEOUT" -eq 0 ]; then
            print_success "All tests passed! 🎉"
            return 0
        else
            print_error "Some tests failed or timed out ❌"
            echo ""
            echo "Troubleshooting tips:"
            echo "1. Check the detailed failure information above"
            echo "2. Review the full logs in test-results/test-report.json"
            echo "3. Verify Docker container memory limits are set correctly"
            echo "4. Check if the RTML library is properly initialized"
            echo "5. Ensure the test environment has sufficient resources"
            return 1
        fi
    else
        print_warning "No test results found"
        print_error "Test execution may have failed completely"
        return 1
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
            echo ""
            echo "Environment:"
            echo "  The script will automatically detect Go version and Docker availability."
            echo "  Test results are saved to test-results/test-report.json"
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
