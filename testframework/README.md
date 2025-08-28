# Go RTML Test Framework

This test framework provides a containerized testing environment for the Go RTML (Runtime Memory Limit) library. It runs tests in isolated containers with controlled memory limits and monitors the results.

## Overview

The test framework consists of two main components:

1. **Test Framework** (`test-framework/`) - Orchestrates test execution, manages containers, and generates reports
2. **Test Runner** (`test-runner/`) - Runs inside containers and performs memory allocation tests

This framework is designed as a separate Go module that can be used to validate the RTML library's memory limit functionality in isolated containers.

## Key Features

- ✅ **Containerized Testing**: Runs tests in isolated Docker containers with controlled memory limits
- ✅ **Memory Validation**: Comprehensive validation of `GetMemLimitRelatedStats()` return values
- ✅ **Realistic Bounds**: Upper and lower bounds for all memory statistics with appropriate slack
- ✅ **Separate Module**: Independent Go module that doesn't pollute the main library
- ✅ **Easy Setup**: Simple build and run process with Makefile and scripts
- ✅ **Detailed Reporting**: JSON reports with container logs and execution details

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Test Framework│    │   Docker        │    │   Test Runner   │
│   (Orchestrator)│────│   Container     │────│   (Test Logic)  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Test Results  │    │   Memory Limit  │    │   Memory        │
│   & Reports     │    │   Enforcement   │    │   Allocation    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Test Types

### Sanity Check Test
- **Purpose**: Validates that `GetMemLimitRelatedStats()` returns realistic and consistent values
- **Behavior**: Allocates a known amount of memory (50 MB) and validates memory statistics
- **Expected Result**: Success (exit code 0) with all sanity checks passing
- **Configuration**:
  - `ALLOC_SIZE_MB=50` (default)
  - Container limit: 512M
  - Timeout: 60 seconds

**What it validates:**
- MemoryLimit: Must be non-zero (512 MB)
- HeapGoal: Must be non-zero and reasonable (60-100 MB for 50MB allocation)
- HeapLive: Must be between 90%-120% of allocated (45-60 MB for 50MB allocation)
- MappedReady: Must be between HeapLive+2MB and HeapLive+10MB (52-60 MB)
- TotalAlloc: Must be between 90%-120% of allocated (45-60 MB for 50MB allocation)
- TotalFree: Must be ≤5MB (0 MB in normal case)

**Typical Results:**
```
Initial stats: MemoryLimit=512 MB, HeapGoal=4 MB, HeapLive=0 MB, MappedReady=2 MB
Final stats: MemoryLimit=512 MB, HeapGoal=72 MB, HeapLive=50 MB, MappedReady=53 MB, TotalAlloc=50 MB, TotalFree=0 MB
```

## Quick Start

### Prerequisites
- Docker installed and running
- Go 1.24 or later
- Make (optional, for using Makefile)

### Installation

1. **Install dependencies**:
   ```bash
   make deps
   # or manually:
   go mod download
   go mod tidy
   ```

2. **Build the framework**:
   ```bash
   make build
   # or manually:
   go build -o bin/test-framework ./test-framework
   go build -ldflags="-checklinkname=0" -o bin/test-runner ./test-runner
   ```

3. **Build Docker image**:
   ```bash
   make docker-build
   # or manually:
   ./build-docker.sh
   ```

4. **Run tests**:
   ```bash
   make docker-run-tests
   # or manually:
   ./bin/test-framework
   ```

5. **Alternative: Use the script**:
   ```bash
   ./scripts/run-tests.sh
   ```

## Configuration

### Test Configuration

Tests are configured in `test-framework/main.go`:

```go
type TestConfig struct {
    Name             string            `json:"name"`
    Image            string            `json:"image"`
    EnvVars          map[string]string `json:"env_vars"`
    MemoryLimit      string            `json:"memory_limit"`
    TimeoutSeconds   int               `json:"timeout_seconds"`
    ExpectedExitCode int               `json:"expected_exit_code"`
}
```

### Environment Variables

The test runner accepts these environment variables:

- `ALLOC_SIZE_MB`: Amount of memory to allocate in MB (default: 50)

## Results and Reporting

### Test Results Structure

```json
{
  "test_name": "sanity-check-test",
  "status": "passed",
  "duration_seconds": 0.925,
  "exit_code": 0,
  "start_time": "2025-08-25T22:14:31.124978+03:00",
  "end_time": "2025-08-25T22:14:32.050791+03:00",
  "error": "",
  "logs": "Starting sanity check test...",
  "memory_stats": {
    "peak_memory_mb": 0,
    "final_memory_mb": 0
  }
}
```

### Sample Output

```
=== Test Results Summary ===
Total Tests: 1
Passed: 1
Failed: 0
Timeout: 0
Report saved to: test-results/test-report.json
```

### Output Files

- `test-results/test-report.json`: Detailed test results in JSON format
- Console output: Summary of test execution

## Customization

### Adding New Test Types

1. **Add test logic** in `test-runner/main.go`:
   ```go
   func runNewTestType(test SanityTest) {
       // Your test logic here
       // Use rtml.GetMemLimitRelatedStats() to validate memory statistics
   }
   ```

2. **Call the new test function** in the main function:
   ```go
   func main() {
       // Parse environment variables
       test := SanityTest{
           allocSizeMB: getEnvAsIntOrDefault("ALLOC_SIZE_MB", 50),
       }
       
       // Run your new test
       runNewTestType(test)
   }
   ```

3. **Add test configuration** in `test-framework/main.go`:
   ```go
   {
       Name:             "new-test",
       Image:            "go-rtml-test:latest",
       MemoryLimit:      "512M",
       TimeoutSeconds:   60,
       ExpectedExitCode: 0,
       EnvVars: map[string]string{
           "ALLOC_SIZE_MB": "50",
           // Add other environment variables
       },
   }
   ```

### Modifying Container Configuration

Edit the `Dockerfile` to:
- Change base image
- Add additional dependencies
- Modify environment variables
- Change build process

## Troubleshooting

### Common Issues

1. **Docker not running**:
   ```
   Error: failed to create docker client
   ```
   Solution: Start Docker daemon

2. **Permission denied**:
   ```
   Error: permission denied while trying to connect to the Docker daemon
   ```
   Solution: Add user to docker group or run with sudo

3. **Memory limit not enforced**:
   - Ensure container memory limit is set correctly
   - Check that GOMEMLIMIT is configured appropriately

4. **Test timeouts**:
   - Increase `TimeoutSeconds` in test configuration
   - Check if test is hanging due to memory issues

### Debugging

1. **Enable verbose logging**:
   ```go
   log.SetLevel(log.DebugLevel)
   ```

2. **Check container logs**:
   ```bash
   docker logs <container_id>
   ```

3. **Inspect container stats**:
   ```bash
   docker stats <container_id>
   ```

## Performance Considerations

- **Container overhead**: Each test runs in a separate container, adding ~50-100ms overhead
- **Memory allocation**: The sanity check allocates 50MB in 1MB chunks with 10ms delays
- **Test duration**: Typical runtime is ~1 second for the sanity check
- **Memory validation**: Comprehensive checks ensure realistic memory statistics
- **Sequential execution**: Currently runs one test at a time for simplicity

## Security

- Containers run with minimal privileges
- Auto-removal ensures no persistent containers
- Memory limits prevent resource exhaustion
- Timeout limits prevent hanging tests

## Contributing

When adding new tests:

1. Follow the existing test structure in `test-runner/main.go`
2. Add appropriate error handling and validation
3. Include meaningful log messages with emojis (✅, ❌, 🎉)
4. Update documentation and examples
5. Test with various memory allocation sizes
6. Ensure proper cleanup and memory management
7. Use `rtml.GetMemLimitRelatedStats()` for validation
8. Keep chunks alive to prevent premature garbage collection

## License

This test framework is part of the go-rtml project and follows the same license terms.
