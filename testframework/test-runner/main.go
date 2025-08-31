package main

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	rtml "github.com/odigos-io/go-rtml"
)

type SanityTest struct {
	allocSizeMB uint64
}

// Global variable to keep chunks alive
var globalChunks [][]byte

func main() {
	// Set up logging with timestamps
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// Parse environment variables
	test := SanityTest{
		allocSizeMB: uint64(getEnvAsIntOrDefault("ALLOC_SIZE_MB", 50)),
	}

	log.Printf("=== Starting sanity check test ===")
	log.Printf("Go version: %s", runtime.Version())
	log.Printf("Allocation size: %d MB", test.allocSizeMB)
	log.Printf("Available CPUs: %d", runtime.NumCPU())
	log.Printf("Initial memory stats:")

	// Log initial memory stats
	var initialMemStats runtime.MemStats
	runtime.ReadMemStats(&initialMemStats)
	log.Printf("  HeapAlloc: %d MB", bytesToMB(initialMemStats.HeapAlloc))
	log.Printf("  HeapSys: %d MB", bytesToMB(initialMemStats.HeapSys))
	log.Printf("  HeapIdle: %d MB", bytesToMB(initialMemStats.HeapIdle))
	log.Printf("  HeapInuse: %d MB", bytesToMB(initialMemStats.HeapInuse))

	// Run the sanity check test
	startTime := time.Now()
	runSanityCheckTest(test)
	duration := time.Since(startTime)

	log.Printf("=== Test completed successfully in %v ===", duration)
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
	log.Printf("Initial RTML stats:")
	log.Printf("  MemoryLimit: %d MB", bytesToMB(initialStats.MemoryLimit))
	log.Printf("  HeapGoal: %d MB", bytesToMB(initialStats.HeapGoal))
	log.Printf("  HeapLive: %d MB", bytesToMB(initialStats.HeapLive))
	log.Printf("  MappedReady: %d MB", bytesToMB(initialStats.MappedReady))
	log.Printf("  TotalAlloc: %d MB", bytesToMB(initialStats.TotalAlloc))
	log.Printf("  TotalFree: %d MB", bytesToMB(initialStats.TotalFree))

	// Allocate memory gradually
	allocSizeBytes := mbToBytes(test.allocSizeMB)
	chunkSize := uint64(256 * 1024) // 256KB chunks for more frequent allocation
	numChunks := allocSizeBytes / chunkSize
	globalChunks = make([][]byte, 0, numChunks)

	log.Printf("Allocating %d MB in %d KB chunks...", test.allocSizeMB, chunkSize/1024)

	allocationStart := time.Now()
	for i := uint64(0); i < numChunks; i++ {
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
				bytesToMB(stats.HeapLive),
				bytesToMB(stats.MappedReady))
		}
	}

	allocationDuration := time.Since(allocationStart)
	log.Printf("Successfully allocated %d MB in %v", test.allocSizeMB, allocationDuration)

	// Keep the chunks alive by doing some work with them
	totalBytes := 0
	for i, chunk := range globalChunks {
		totalBytes += len(chunk)
		// Access some bytes to ensure they're not optimized away
		if i%10 == 0 && len(chunk) > 0 {
			_ = chunk[0]
		}
	}
	log.Printf("Verified %d chunks with total size: %d MB", len(globalChunks), bytesToMB(uint64(totalBytes)))

	// Keep chunks alive by storing them globally
	log.Printf("Stored %d chunks in global variable to prevent GC", len(globalChunks))

	// Force physical memory commit by touching all bytes
	forceMemoryCommit(globalChunks)

	// Make some computation that touches all of the global chunks to make sure they are not optimized away
	foo := 0
	for _, chunk := range globalChunks {
		for _, val := range chunk {
			foo += int(val)
		}
	}
	log.Printf("Computed checksum across all chunks: %d", foo)

	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)
	log.Printf("Final runtime stats:")
	log.Printf("  HeapAlloc: %d MB", bytesToMB(finalMemStats.HeapAlloc))
	log.Printf("  HeapSys: %d MB", bytesToMB(finalMemStats.HeapSys))
	log.Printf("  HeapIdle: %d MB", bytesToMB(finalMemStats.HeapIdle))
	log.Printf("  HeapInuse: %d MB", bytesToMB(finalMemStats.HeapInuse))

	// Get final stats
	finalStats := rtml.GetMemLimitRelatedStats()
	log.Printf("Final RTML stats:")
	log.Printf("  MemoryLimit: %d MB", bytesToMB(finalStats.MemoryLimit))
	log.Printf("  HeapGoal: %d MB", bytesToMB(finalStats.HeapGoal))
	log.Printf("  HeapLive: %d MB", bytesToMB(finalStats.HeapLive))
	log.Printf("  MappedReady: %d MB", bytesToMB(finalStats.MappedReady))
	log.Printf("  TotalAlloc: %d MB", bytesToMB(finalStats.TotalAlloc))
	log.Printf("  TotalFree: %d MB", bytesToMB(finalStats.TotalFree))

	// Sanity checks with detailed error messages
	log.Println("Performing sanity checks...")

	// Check that MemoryLimit is not zero
	if finalStats.MemoryLimit == 0 {
		log.Printf("❌ FAIL: MemoryLimit is zero - RTML is not properly detecting memory limits")
		os.Exit(1)
	}
	log.Printf("✅ MemoryLimit is valid: %d MB", bytesToMB(finalStats.MemoryLimit))

	// Check that HeapGoal is not zero
	if finalStats.HeapGoal == 0 {
		log.Printf("❌ FAIL: HeapGoal is zero - RTML is not calculating heap goals properly")
		os.Exit(1)
	}
	log.Printf("✅ HeapGoal is valid: %d MB", bytesToMB(finalStats.HeapGoal))

	// Check that HeapLive increased after allocation
	if finalStats.HeapLive <= initialStats.HeapLive {
		log.Printf("❌ FAIL: HeapLive did not increase after allocation")
		log.Printf("   Initial: %d MB", bytesToMB(initialStats.HeapLive))
		log.Printf("   Final: %d MB", bytesToMB(finalStats.HeapLive))
		os.Exit(1)
	}
	log.Printf("✅ HeapLive increased: %d MB -> %d MB",
		bytesToMB(initialStats.HeapLive), bytesToMB(finalStats.HeapLive))

	// Check that MappedReady is not zero
	if finalStats.MappedReady == 0 {
		log.Printf("❌ FAIL: MappedReady is zero - No memory pages are mapped and ready")
		os.Exit(1)
	}
	log.Printf("✅ MappedReady is valid: %d MB", bytesToMB(finalStats.MappedReady))

	// Check that TotalAlloc increased
	if finalStats.TotalAlloc <= initialStats.TotalAlloc {
		log.Printf("❌ FAIL: TotalAlloc did not increase")
		log.Printf("   Initial: %d MB", bytesToMB(initialStats.TotalAlloc))
		log.Printf("   Final: %d MB", bytesToMB(finalStats.TotalAlloc))
		os.Exit(1)
	}
	log.Printf("✅ TotalAlloc increased: %d MB -> %d MB",
		bytesToMB(initialStats.TotalAlloc), bytesToMB(finalStats.TotalAlloc))

	// Check that HeapLive is reasonable (should be between 90% and 120% of allocated memory)
	expectedMinHeapLive := mbToBytes(test.allocSizeMB) * 9 / 10  // 90% of allocated
	expectedMaxHeapLive := mbToBytes(test.allocSizeMB) * 12 / 10 // 120% of allocated
	if finalStats.HeapLive < expectedMinHeapLive {
		log.Printf("❌ FAIL: HeapLive too low")
		log.Printf("   Expected at least: %d MB", bytesToMB(expectedMinHeapLive))
		log.Printf("   Got: %d MB", bytesToMB(finalStats.HeapLive))
		os.Exit(1)
	}
	if finalStats.HeapLive > expectedMaxHeapLive {
		log.Printf("❌ FAIL: HeapLive too high")
		log.Printf("   Expected at most: %d MB", bytesToMB(expectedMaxHeapLive))
		log.Printf("   Got: %d MB", bytesToMB(finalStats.HeapLive))
		os.Exit(1)
	}
	log.Printf("✅ HeapLive is reasonable: %d MB (allocated %d MB, expected %d-%d MB)",
		bytesToMB(finalStats.HeapLive), test.allocSizeMB,
		bytesToMB(expectedMinHeapLive), bytesToMB(expectedMaxHeapLive))

	// Check that MappedReady is reasonable (should be between HeapLive + 2MB and HeapLive + 10MB)
	expectedMinMappedReady := finalStats.HeapLive + mbToBytes(2)  // HeapLive + 2MB overhead
	expectedMaxMappedReady := finalStats.HeapLive + mbToBytes(10) // HeapLive + 10MB max overhead
	if finalStats.MappedReady < expectedMinMappedReady {
		log.Printf("❌ FAIL: MappedReady too low")
		log.Printf("   Expected at least: %d MB", bytesToMB(expectedMinMappedReady))
		log.Printf("   Got: %d MB", bytesToMB(finalStats.MappedReady))
		log.Printf("   HeapLive: %d MB", bytesToMB(finalStats.HeapLive))
		os.Exit(1)
	}
	if finalStats.MappedReady > expectedMaxMappedReady {
		log.Printf("❌ FAIL: MappedReady too high")
		log.Printf("   Expected at most: %d MB", bytesToMB(expectedMaxMappedReady))
		log.Printf("   Got: %d MB", bytesToMB(finalStats.MappedReady))
		log.Printf("   HeapLive: %d MB", bytesToMB(finalStats.HeapLive))
		log.Printf("   This could indicate:")
		log.Printf("   - Excessive memory mapping overhead")
		log.Printf("   - Memory fragmentation")
		os.Exit(1)
	}
	log.Printf("✅ MappedReady is reasonable: %d MB (HeapLive: %d MB, expected %d-%d MB)",
		bytesToMB(finalStats.MappedReady), bytesToMB(finalStats.HeapLive),
		bytesToMB(expectedMinMappedReady), bytesToMB(expectedMaxMappedReady))

	// Check that HeapGoal is reasonable (should be between HeapLive and HeapLive + 60MB)
	expectedMinHeapGoal := finalStats.HeapLive                 // HeapGoal should be at least HeapLive
	expectedMaxHeapGoal := finalStats.HeapLive + mbToBytes(60) // HeapLive + 60MB max growth allowance
	if finalStats.HeapGoal < expectedMinHeapGoal {
		log.Printf("❌ FAIL: HeapGoal too low")
		log.Printf("   Expected at least: %d MB", bytesToMB(expectedMinHeapGoal))
		log.Printf("   Got: %d MB", bytesToMB(finalStats.HeapGoal))
		log.Printf("   HeapLive: %d MB", bytesToMB(finalStats.HeapLive))
		os.Exit(1)
	}
	if finalStats.HeapGoal > expectedMaxHeapGoal {
		log.Printf("❌ FAIL: HeapGoal too high")
		log.Printf("   Expected at most: %d MB", bytesToMB(expectedMaxHeapGoal))
		log.Printf("   Got: %d MB", bytesToMB(finalStats.HeapGoal))
		log.Printf("   HeapLive: %d MB", bytesToMB(finalStats.HeapLive))
		os.Exit(1)
	}
	log.Printf("✅ HeapGoal is reasonable: %d MB (HeapLive: %d MB, expected %d-%d MB)",
		bytesToMB(finalStats.HeapGoal), bytesToMB(finalStats.HeapLive),
		bytesToMB(expectedMinHeapGoal), bytesToMB(expectedMaxHeapGoal))

	// Check that TotalAlloc is reasonable (should be between 90% and 120% of allocated amount)
	expectedMinTotalAlloc := mbToBytes(test.allocSizeMB) * 9 / 10  // 90% of allocated
	expectedMaxTotalAlloc := mbToBytes(test.allocSizeMB) * 12 / 10 // 120% of allocated
	if finalStats.TotalAlloc < expectedMinTotalAlloc {
		log.Printf("❌ FAIL: TotalAlloc too low")
		log.Printf("   Expected at least: %d MB", bytesToMB(expectedMinTotalAlloc))
		log.Printf("   Got: %d MB", bytesToMB(finalStats.TotalAlloc))
		os.Exit(1)
	}
	if finalStats.TotalAlloc > expectedMaxTotalAlloc {
		log.Printf("❌ FAIL: TotalAlloc too high")
		log.Printf("   Expected at most: %d MB", bytesToMB(expectedMaxTotalAlloc))
		log.Printf("   Got: %d MB", bytesToMB(finalStats.TotalAlloc))
		os.Exit(1)
	}
	log.Printf("✅ TotalAlloc is reasonable: %d MB (allocated %d MB, expected %d-%d MB)",
		bytesToMB(finalStats.TotalAlloc), test.allocSizeMB,
		bytesToMB(expectedMinTotalAlloc), bytesToMB(expectedMaxTotalAlloc))

	// Check that TotalFree is reasonable (should be 0 or very small for our test)
	expectedMaxTotalFree := mbToBytes(5) // 5MB max
	if finalStats.TotalFree > expectedMaxTotalFree {
		log.Printf("❌ FAIL: TotalFree too high")
		log.Printf("   Expected at most: %d MB", bytesToMB(expectedMaxTotalFree))
		log.Printf("   Got: %d MB", bytesToMB(finalStats.TotalFree))
		os.Exit(1)
	}
	log.Printf("✅ TotalFree is reasonable: %d MB", bytesToMB(finalStats.TotalFree))

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

func bytesToMB(bytes uint64) uint64 {
	return bytes / (1024 * 1024)
}

func mbToBytes(mb uint64) uint64 {
	return mb * 1024 * 1024
}
