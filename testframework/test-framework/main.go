package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type TestResult struct {
	TestName    string    `json:"test_name"`
	Status      string    `json:"status"` // "passed", "failed", "timeout"
	Duration    float64   `json:"duration_seconds"`
	ExitCode    int       `json:"exit_code"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Error       string    `json:"error,omitempty"`
	Logs        string    `json:"logs,omitempty"`
	MemoryStats struct {
		PeakMemoryMB  float64 `json:"peak_memory_mb"`
		FinalMemoryMB float64 `json:"final_memory_mb"`
	} `json:"memory_stats"`
}

type TestConfig struct {
	Name             string            `json:"name"`
	Image            string            `json:"image"`
	EnvVars          map[string]string `json:"env_vars"`
	MemoryLimit      string            `json:"memory_limit"`
	TimeoutSeconds   int               `json:"timeout_seconds"`
	ExpectedExitCode int               `json:"expected_exit_code"`
}

type TestRunner struct {
	dockerClient *client.Client
	results      []TestResult
}

func NewTestRunner() (*TestRunner, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &TestRunner{
		dockerClient: dockerClient,
		results:      make([]TestResult, 0),
	}, nil
}

func (tr *TestRunner) RunTest(ctx context.Context, config TestConfig) TestResult {
	result := TestResult{
		TestName:  config.Name,
		StartTime: time.Now(),
	}

	log.Printf("Starting test: %s", config.Name)

	// Create container config
	containerConfig := &container.Config{
		Image: config.Image,
		Env:   tr.buildEnvVars(config.EnvVars),
		Cmd:   []string{"/app/test-runner"},
	}

	// Create host config with memory limit
	hostConfig := &container.HostConfig{
		AutoRemove: true,
		Resources: container.Resources{
			Memory: tr.parseMemoryLimit(config.MemoryLimit),
		},
	}

	// Create container
	resp, err := tr.dockerClient.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to create container: %v", err)
		result.EndTime = time.Now()
		return result
	}

	containerID := resp.ID
	defer func() {
		// Clean up container if it's still running
		tr.dockerClient.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true})
	}()

	// Start container
	if err := tr.dockerClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{}); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to start container: %v", err)
		result.EndTime = time.Now()
		return result
	}

	// Start collecting memory stats in background
	statsCtx, statsCancel := context.WithCancel(ctx)
	defer statsCancel()

	var peakMemory uint64
	var finalMemory uint64
	var statsCollected bool

	go func() {
		// Try to get stats multiple times to ensure we capture some data
		for i := 0; i < 10; i++ {
			stats, err := tr.dockerClient.ContainerStats(statsCtx, containerID, false)
			if err != nil {
				log.Printf("Failed to get container stats (attempt %d): %v", i+1, err)
				time.Sleep(50 * time.Millisecond)
				continue
			}

			var containerStats types.StatsJSON
			if err := json.NewDecoder(stats.Body).Decode(&containerStats); err != nil {
				log.Printf("Failed to decode stats (attempt %d): %v", i+1, err)
				stats.Body.Close()
				time.Sleep(50 * time.Millisecond)
				continue
			}
			stats.Body.Close()

			// Use RSS (Resident Set Size) if available, otherwise fall back to Usage
			var usage uint64
			if rss, exists := containerStats.MemoryStats.Stats["rss"]; exists && rss > 0 {
				usage = rss
			} else if containerStats.MemoryStats.Usage > 0 {
				usage = containerStats.MemoryStats.Usage
			} else {
				// If both are 0, try to get from cache stats
				if cache, exists := containerStats.MemoryStats.Stats["cache"]; exists {
					if rss, exists := containerStats.MemoryStats.Stats["rss"]; exists {
						usage = cache + rss
					} else {
						usage = cache
					}
				}
			}

			if usage > 0 {
				statsCollected = true
				if usage > peakMemory {
					peakMemory = usage
				}
				finalMemory = usage
				log.Printf("Memory stats collected (attempt %d): usage=%d bytes (%.2f MB), peak=%d bytes (%.2f MB)",
					i+1, usage, float64(usage)/(1024*1024), peakMemory, float64(peakMemory)/(1024*1024))
			}

			// Wait a bit before next attempt
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// Give some time for initial stats collection
	time.Sleep(200 * time.Millisecond)

	// Wait for container to finish with timeout
	timeout := time.Duration(config.TimeoutSeconds) * time.Second
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	waitCh, errCh := tr.dockerClient.ContainerWait(waitCtx, containerID, container.WaitConditionNotRunning)

	select {
	case waitResult := <-waitCh:
		result.ExitCode = int(waitResult.StatusCode)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime).Seconds()

		// Get container logs
		logs, err := tr.dockerClient.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
		if err == nil {
			defer logs.Close()
			// Read logs (simplified - in production you'd want to handle this more robustly)
			buf := make([]byte, 4096)
			n, _ := logs.Read(buf)
			if n > 0 {
				result.Logs = string(buf[:n])
			}
		}

		// Set collected memory stats
		result.MemoryStats.PeakMemoryMB = float64(peakMemory) / (1024 * 1024)
		result.MemoryStats.FinalMemoryMB = float64(finalMemory) / (1024 * 1024)

		if !statsCollected {
			log.Printf("Warning: No memory stats were collected for test %s", config.Name)
		} else {
			log.Printf("Memory stats for test %s: peak=%.2f MB, final=%.2f MB",
				config.Name, result.MemoryStats.PeakMemoryMB, result.MemoryStats.FinalMemoryMB)
		}

		// Determine test status
		if result.ExitCode == config.ExpectedExitCode {
			result.Status = "passed"
		} else {
			result.Status = "failed"
			result.Error = fmt.Sprintf("expected exit code %d, got %d", config.ExpectedExitCode, result.ExitCode)
		}

	case err := <-errCh:
		result.Status = "failed"
		result.Error = fmt.Sprintf("container wait error: %v", err)
		result.EndTime = time.Now()

	case <-waitCtx.Done():
		result.Status = "timeout"
		result.Error = "test timed out"
		result.EndTime = time.Now()
		result.Duration = timeout.Seconds()
	}

	log.Printf("Test %s completed with status: %s", config.Name, result.Status)
	return result
}

func (tr *TestRunner) buildEnvVars(envVars map[string]string) []string {
	var env []string
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

func (tr *TestRunner) parseMemoryLimit(limit string) int64 {
	// Simple memory parsing - in production you'd want more robust parsing
	var multiplier int64 = 1
	if len(limit) > 0 {
		switch limit[len(limit)-1] {
		case 'G', 'g':
			multiplier = 1024 * 1024 * 1024
			limit = limit[:len(limit)-1]
		case 'M', 'm':
			multiplier = 1024 * 1024
			limit = limit[:len(limit)-1]
		case 'K', 'k':
			multiplier = 1024
			limit = limit[:len(limit)-1]
		}
	}

	var value int64
	fmt.Sscanf(limit, "%d", &value)
	return value * multiplier
}

func (tr *TestRunner) RunTestSuite(ctx context.Context, configs []TestConfig) {
	for _, config := range configs {
		result := tr.RunTest(ctx, config)
		tr.results = append(tr.results, result)
	}
}

func (tr *TestRunner) GenerateReport() {
	// Create results directory
	resultsDir := "test-results"
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		log.Printf("Failed to create results directory: %v", err)
		return
	}

	// Generate JSON report
	reportPath := filepath.Join(resultsDir, "test-report.json")
	reportData, err := json.MarshalIndent(tr.results, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal report: %v", err)
		return
	}

	if err := os.WriteFile(reportPath, reportData, 0644); err != nil {
		log.Printf("Failed to write report: %v", err)
		return
	}

	// Generate summary
	passed := 0
	failed := 0
	timeout := 0

	for _, result := range tr.results {
		switch result.Status {
		case "passed":
			passed++
		case "failed":
			failed++
		case "timeout":
			timeout++
		}
	}

	fmt.Printf("\n=== Test Results Summary ===\n")
	fmt.Printf("Total Tests: %d\n", len(tr.results))
	fmt.Printf("Passed: %d\n", passed)
	fmt.Printf("Failed: %d\n", failed)
	fmt.Printf("Timeout: %d\n", timeout)
	fmt.Printf("Report saved to: %s\n", reportPath)
}

func main() {
	// Define single sanity check test configuration
	testConfigs := []TestConfig{
		{
			Name:             "sanity-check-test",
			Image:            "go-rtml-test:latest",
			MemoryLimit:      "512M",
			TimeoutSeconds:   60,
			ExpectedExitCode: 0,
			EnvVars: map[string]string{
				"ALLOC_SIZE_MB": "50",
			},
		},
	}

	runner, err := NewTestRunner()
	if err != nil {
		log.Fatalf("Failed to create test runner: %v", err)
	}

	ctx := context.Background()
	runner.RunTestSuite(ctx, testConfigs)
	runner.GenerateReport()
}
