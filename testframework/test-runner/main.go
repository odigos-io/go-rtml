package main

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	rtml "github.com/odigos-io/go-rtml"
)

type SanityTest struct {
	allocSizeMB int
}

// Global variable to keep chunks alive
var globalChunks [][]byte

func main() {
	// Parse environment variables
	test := SanityTest{
		allocSizeMB: getEnvAsIntOrDefault("ALLOC_SIZE_MB", 50),
	}

	log.Printf("Starting sanity check test")
	log.Printf("Allocation size: %d MB", test.allocSizeMB)

	// Run the sanity check test
	runSanityCheckTest(test)
}

func forceMemoryCommit(chunks [][]byte) {
	log.Println("Forcing physical memory commit by touching all allocated bytes...")
	var totalChecksum uint64
	for i, chunk := range chunks {
		// Touch every byte to force page commit to RSS
		var chunkChecksum uint64
		for j := 0; j < len(chunk); j++ {
			chunk[j] = byte(i%256 + 1)
			// Force memory barrier every 4KB to ensure page commit
			if j%4096 == 0 {
				chunkChecksum += uint64(chunk[j]) // Read back and use the value
			}
		}
		totalChecksum += chunkChecksum

		// Force a second pass to ensure pages are committed
		for j := 0; j < len(chunk); j += 4096 {
			chunkChecksum += uint64(chunk[j]) // Read every page again
		}

		// Try to lock this chunk in memory (mlock)
		if len(chunk) > 0 {
			// Note: This might fail due to container restrictions, but worth trying
			syscall.Mlock(chunk)
		}

		if i%10 == 0 {
			log.Printf("Touched chunk %d/%d", i+1, len(chunks))
		}
	}
	// Use total checksum to prevent optimization
	log.Printf("Total checksum: %d", totalChecksum)

	// Try to read process memory stats from /proc/self/status
	if data, err := os.ReadFile("/proc/self/status"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "VmRSS:") {
				log.Printf("Process RSS from /proc/self/status: %s", line)
			}
			if strings.HasPrefix(line, "VmSize:") {
				log.Printf("Process VmSize from /proc/self/status: %s", line)
			}
		}
	}

	log.Println("Memory commit complete")
}

func runSanityCheckTest(test SanityTest) {
	log.Println("Running sanity check test...")

	// Get initial stats
	initialStats := rtml.GetMemLimitRelatedStats()
	log.Printf("Initial stats: MemoryLimit=%d MB, HeapGoal=%d MB, HeapLive=%d MB, MappedReady=%d MB",
		initialStats.MemoryLimit/(1024*1024),
		initialStats.HeapGoal/(1024*1024),
		initialStats.HeapLive/(1024*1024),
		initialStats.MappedReady/(1024*1024))

	// Allocate memory gradually
	allocSizeBytes := test.allocSizeMB * 1024 * 1024
	chunkSize := 256 * 1024 // 256KB chunks for more frequent allocation
	globalChunks = make([][]byte, 0, allocSizeBytes/chunkSize)

	log.Printf("Allocating %d MB in %d KB chunks...", test.allocSizeMB, chunkSize/1024)

	for i := 0; i < allocSizeBytes/chunkSize; i++ {
		chunk := make([]byte, chunkSize)

		// Force RSS by touching every page in the chunk
		// This ensures the memory is actually committed to physical RAM
		var checksum uint64
		for j := 0; j < len(chunk); j++ {
			chunk[j] = byte(i%256 + 1)
			// Force memory barrier every 4KB to ensure page commit
			if j%4096 == 0 {
				checksum += uint64(chunk[j]) // Read back and use the value
			}
		}
		// Use checksum to prevent optimization
		if checksum == 0 {
			log.Printf("Warning: checksum is zero for chunk %d", i)
		}

		// Force a second pass to ensure pages are committed
		for j := 0; j < len(chunk); j += 4096 {
			checksum += uint64(chunk[j]) // Read every page again
		}

		globalChunks = append(globalChunks, chunk)

		// Log progress every 10 chunks
		if i%10 == 0 {
			stats := rtml.GetMemLimitRelatedStats()
			log.Printf("Progress: chunk %d/%d, HeapLive=%d MB, MappedReady=%d MB",
				i+1, allocSizeBytes/chunkSize,
				stats.HeapLive/(1024*1024),
				stats.MappedReady/(1024*1024))
		}

		// Force garbage collection more frequently
		if i%2 == 0 {
			// Force GC and read memory stats to ensure RSS commitment
			runtime.GC()
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			log.Printf("GC triggered: HeapAlloc=%d MB, HeapSys=%d MB",
				m.HeapAlloc/(1024*1024), m.HeapSys/(1024*1024))
		}
	}

	log.Printf("Successfully allocated %d MB", test.allocSizeMB)

	// Keep the chunks alive by doing some work with them
	totalBytes := 0
	for i, chunk := range globalChunks {
		totalBytes += len(chunk)
		// Access some bytes to ensure they're not optimized away
		if i%10 == 0 && len(chunk) > 0 {
			_ = chunk[0]
		}
	}
	log.Printf("Verified %d chunks with total size: %d MB", len(globalChunks), totalBytes/(1024*1024))

	// Keep chunks alive by storing them globally
	log.Printf("Stored %d chunks in global variable to prevent GC", len(globalChunks))

	// Force physical memory commit by touching all bytes
	forceMemoryCommit(globalChunks)

	// Force final GC and dump runtime memory stats
	runtime.GC()

	// Make some computation that touches all of the global chunks to make sure they are not optimized away
	foo := 0
	for _, chunk := range globalChunks {
		for _, val := range chunk {
			foo += int(val)
		}
	}

	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)
	log.Printf("Final runtime stats: HeapAlloc=%d MB, HeapSys=%d MB, HeapIdle=%d MB, HeapInuse=%d MB",
		finalMemStats.HeapAlloc/(1024*1024), finalMemStats.HeapSys/(1024*1024),
		finalMemStats.HeapIdle/(1024*1024), finalMemStats.HeapInuse/(1024*1024))

	// Get final stats
	finalStats := rtml.GetMemLimitRelatedStats()
	log.Printf("Final stats: MemoryLimit=%d MB, HeapGoal=%d MB, HeapLive=%d MB, MappedReady=%d MB, TotalAlloc=%d MB, TotalFree=%d MB",
		finalStats.MemoryLimit/(1024*1024),
		finalStats.HeapGoal/(1024*1024),
		finalStats.HeapLive/(1024*1024),
		finalStats.MappedReady/(1024*1024),
		finalStats.TotalAlloc/(1024*1024),
		finalStats.TotalFree/(1024*1024))

	// Sanity checks
	log.Println("Performing sanity checks...")

	// Check that MemoryLimit is not zero
	if finalStats.MemoryLimit == 0 {
		log.Printf("❌ FAIL: MemoryLimit is zero")
		os.Exit(1)
	}
	log.Printf("✅ MemoryLimit is valid: %d MB", finalStats.MemoryLimit/(1024*1024))

	// Check that HeapGoal is not zero
	if finalStats.HeapGoal == 0 {
		log.Printf("❌ FAIL: HeapGoal is zero")
		os.Exit(1)
	}
	log.Printf("✅ HeapGoal is valid: %d MB", finalStats.HeapGoal/(1024*1024))

	// Check that HeapLive increased after allocation
	if finalStats.HeapLive <= initialStats.HeapLive {
		log.Printf("❌ FAIL: HeapLive did not increase. Initial: %d MB, Final: %d MB",
			initialStats.HeapLive/(1024*1024), finalStats.HeapLive/(1024*1024))
		os.Exit(1)
	}
	log.Printf("✅ HeapLive increased: %d MB -> %d MB",
		initialStats.HeapLive/(1024*1024), finalStats.HeapLive/(1024*1024))

	// Check that MappedReady is not zero
	if finalStats.MappedReady == 0 {
		log.Printf("❌ FAIL: MappedReady is zero")
		os.Exit(1)
	}
	log.Printf("✅ MappedReady is valid: %d MB", finalStats.MappedReady/(1024*1024))

	// Check that TotalAlloc increased
	if finalStats.TotalAlloc <= initialStats.TotalAlloc {
		log.Printf("❌ FAIL: TotalAlloc did not increase. Initial: %d MB, Final: %d MB",
			initialStats.TotalAlloc/(1024*1024), finalStats.TotalAlloc/(1024*1024))
		os.Exit(1)
	}
	log.Printf("✅ TotalAlloc increased: %d MB -> %d MB",
		initialStats.TotalAlloc/(1024*1024), finalStats.TotalAlloc/(1024*1024))

	// Check that HeapLive is reasonable (should be between 90% and 120% of allocated memory)
	expectedMinHeapLive := uint64(test.allocSizeMB) * 1024 * 1024 * 9 / 10  // 90% of allocated
	expectedMaxHeapLive := uint64(test.allocSizeMB) * 1024 * 1024 * 12 / 10 // 120% of allocated
	if finalStats.HeapLive < expectedMinHeapLive {
		log.Printf("❌ FAIL: HeapLive too low. Expected at least %d MB, got %d MB",
			expectedMinHeapLive/(1024*1024), finalStats.HeapLive/(1024*1024))
		os.Exit(1)
	}
	if finalStats.HeapLive > expectedMaxHeapLive {
		log.Printf("❌ FAIL: HeapLive too high. Expected at most %d MB, got %d MB",
			expectedMaxHeapLive/(1024*1024), finalStats.HeapLive/(1024*1024))
		os.Exit(1)
	}
	log.Printf("✅ HeapLive is reasonable: %d MB (allocated %d MB, expected %d-%d MB)",
		finalStats.HeapLive/(1024*1024), test.allocSizeMB,
		expectedMinHeapLive/(1024*1024), expectedMaxHeapLive/(1024*1024))

	// Check that MappedReady is reasonable (should be between HeapLive + 2MB and HeapLive + 10MB)
	expectedMinMappedReady := finalStats.HeapLive + 2*1024*1024  // HeapLive + 2MB overhead
	expectedMaxMappedReady := finalStats.HeapLive + 10*1024*1024 // HeapLive + 10MB max overhead
	if finalStats.MappedReady < expectedMinMappedReady {
		log.Printf("❌ FAIL: MappedReady too low. Expected at least %d MB, got %d MB",
			expectedMinMappedReady/(1024*1024), finalStats.MappedReady/(1024*1024))
		os.Exit(1)
	}
	if finalStats.MappedReady > expectedMaxMappedReady {
		log.Printf("❌ FAIL: MappedReady too high. Expected at most %d MB, got %d MB",
			expectedMaxMappedReady/(1024*1024), finalStats.MappedReady/(1024*1024))
		os.Exit(1)
	}
	log.Printf("✅ MappedReady is reasonable: %d MB (HeapLive: %d MB, expected %d-%d MB)",
		finalStats.MappedReady/(1024*1024), finalStats.HeapLive/(1024*1024),
		expectedMinMappedReady/(1024*1024), expectedMaxMappedReady/(1024*1024))

	// Check that HeapGoal is reasonable (should be between HeapLive and HeapLive + 60MB)
	expectedMinHeapGoal := finalStats.HeapLive                // HeapGoal should be at least HeapLive
	expectedMaxHeapGoal := finalStats.HeapLive + 60*1024*1024 // HeapLive + 60MB max growth allowance
	if finalStats.HeapGoal < expectedMinHeapGoal {
		log.Printf("❌ FAIL: HeapGoal too low. Expected at least %d MB, got %d MB",
			expectedMinHeapGoal/(1024*1024), finalStats.HeapGoal/(1024*1024))
		os.Exit(1)
	}
	if finalStats.HeapGoal > expectedMaxHeapGoal {
		log.Printf("❌ FAIL: HeapGoal too high. Expected at most %d MB, got %d MB",
			expectedMaxHeapGoal/(1024*1024), finalStats.HeapGoal/(1024*1024))
		os.Exit(1)
	}
	log.Printf("✅ HeapGoal is reasonable: %d MB (HeapLive: %d MB, expected %d-%d MB)",
		finalStats.HeapGoal/(1024*1024), finalStats.HeapLive/(1024*1024),
		expectedMinHeapGoal/(1024*1024), expectedMaxHeapGoal/(1024*1024))

	// Check that TotalAlloc is reasonable (should be between 90% and 120% of allocated amount)
	expectedMinTotalAlloc := uint64(test.allocSizeMB) * 1024 * 1024 * 9 / 10  // 90% of allocated
	expectedMaxTotalAlloc := uint64(test.allocSizeMB) * 1024 * 1024 * 12 / 10 // 120% of allocated
	if finalStats.TotalAlloc < expectedMinTotalAlloc {
		log.Printf("❌ FAIL: TotalAlloc too low. Expected at least %d MB, got %d MB",
			expectedMinTotalAlloc/(1024*1024), finalStats.TotalAlloc/(1024*1024))
		os.Exit(1)
	}
	if finalStats.TotalAlloc > expectedMaxTotalAlloc {
		log.Printf("❌ FAIL: TotalAlloc too high. Expected at most %d MB, got %d MB",
			expectedMaxTotalAlloc/(1024*1024), finalStats.TotalAlloc/(1024*1024))
		os.Exit(1)
	}
	log.Printf("✅ TotalAlloc is reasonable: %d MB (allocated %d MB, expected %d-%d MB)",
		finalStats.TotalAlloc/(1024*1024), test.allocSizeMB,
		expectedMinTotalAlloc/(1024*1024), expectedMaxTotalAlloc/(1024*1024))

	// Check that TotalFree is reasonable (should be 0 or very small for our test)
	expectedMaxTotalFree := uint64(5) * 1024 * 1024 // 5MB max
	if finalStats.TotalFree > expectedMaxTotalFree {
		log.Printf("❌ FAIL: TotalFree too high. Expected at most %d MB, got %d MB",
			expectedMaxTotalFree/(1024*1024), finalStats.TotalFree/(1024*1024))
		os.Exit(1)
	}
	log.Printf("✅ TotalFree is reasonable: %d MB", finalStats.TotalFree/(1024*1024))

	log.Println("🎉 All sanity checks passed!")
	log.Println("Sanity check test completed successfully")
}

func getEnvAsIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
