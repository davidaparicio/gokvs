package performance

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidaparicio/gokvs/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createRequest creates an http.Request for testing purposes with testing.TB interface
func createRequest(tb testing.TB, method, path, body string) *http.Request {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	
	req, err := http.NewRequest(method, path, bodyReader)
	if err != nil {
		tb.Fatalf("Failed to create request: %v", err)
	}
	
	return req
}

// BenchmarkSingleOperations benchmarks individual operations
func BenchmarkPutOperation(b *testing.B) {
	server, cleanup := helpers.CreateTestServerWithMetricsTB(b)
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-put-%d", i)
		value := fmt.Sprintf("bench-value-%d", i)
		
		req := createRequest(b, "PUT", fmt.Sprintf("/v1/%s", key), value)
		resp := httptest.NewRecorder()
		server.ServeHTTP(resp, req)
		
		if resp.Code != http.StatusCreated {
			b.Errorf("PUT failed with status %d", resp.Code)
		}
	}
}

func BenchmarkGetOperation(b *testing.B) {
	server, cleanup := helpers.CreateTestServerWithMetricsTB(b)
	defer cleanup()

	// Pre-populate with data
	numKeys := 1000
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("bench-get-%d", i)
		value := fmt.Sprintf("bench-value-%d", i)
		
		req := createRequest(b, "PUT", fmt.Sprintf("/v1/%s", key), value)
		resp := httptest.NewRecorder()
		server.ServeHTTP(resp, req)
		
		if resp.Code != http.StatusCreated {
			b.Fatalf("Setup failed: PUT returned %d", resp.Code)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-get-%d", i%numKeys)
		
		req := createRequest(b, "GET", fmt.Sprintf("/v1/%s", key), "")
		resp := httptest.NewRecorder()
		server.ServeHTTP(resp, req)
		
		if resp.Code != http.StatusOK {
			b.Errorf("GET failed with status %d", resp.Code)
		}
	}
}

func BenchmarkDeleteOperation(b *testing.B) {
	server, cleanup := helpers.CreateTestServerWithMetricsTB(b)
	defer cleanup()

	// Pre-populate with data
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-delete-%d", i)
		value := fmt.Sprintf("bench-value-%d", i)
		
		req := createRequest(b, "PUT", fmt.Sprintf("/v1/%s", key), value)
		resp := httptest.NewRecorder()
		server.ServeHTTP(resp, req)
		
		if resp.Code != http.StatusCreated {
			b.Fatalf("Setup failed: PUT returned %d", resp.Code)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-delete-%d", i)
		
		req := createRequest(b, "DELETE", fmt.Sprintf("/v1/%s", key), "")
		resp := httptest.NewRecorder()
		server.ServeHTTP(resp, req)
		
		if resp.Code != http.StatusOK {
			b.Errorf("DELETE failed with status %d", resp.Code)
		}
	}
}

// BenchmarkMixedOperations benchmarks realistic workloads
func BenchmarkMixedWorkload(b *testing.B) {
	server, cleanup := helpers.CreateTestServerWithMetricsTB(b)
	defer cleanup()

	// Pre-populate with some data
	baseKeys := 100
	for i := 0; i < baseKeys; i++ {
		key := fmt.Sprintf("mixed-base-%d", i)
		value := fmt.Sprintf("base-value-%d", i)
		
		req := createRequest(b, "PUT", fmt.Sprintf("/v1/%s", key), value)
		resp := httptest.NewRecorder()
		server.ServeHTTP(resp, req)
		
		if resp.Code != http.StatusCreated {
			b.Fatalf("Setup failed: PUT returned %d", resp.Code)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		switch i % 10 {
		case 0, 1, 2, 3, 4, 5: // 60% reads
			key := fmt.Sprintf("mixed-base-%d", i%baseKeys)
			req := createRequest(b, "GET", fmt.Sprintf("/v1/%s", key), "")
			resp := httptest.NewRecorder()
			server.ServeHTTP(resp, req)
			
		case 6, 7, 8: // 30% writes
			key := fmt.Sprintf("mixed-new-%d", i)
			value := fmt.Sprintf("new-value-%d", i)
			req := createRequest(b, "PUT", fmt.Sprintf("/v1/%s", key), value)
			resp := httptest.NewRecorder()
			server.ServeHTTP(resp, req)
			
		case 9: // 10% deletes
			key := fmt.Sprintf("mixed-base-%d", i%baseKeys)
			req := createRequest(b, "DELETE", fmt.Sprintf("/v1/%s", key), "")
			resp := httptest.NewRecorder()
			server.ServeHTTP(resp, req)
		}
	}
}

// Concurrent benchmarks
func BenchmarkConcurrentPut(b *testing.B) {
	server, cleanup := helpers.CreateTestServerWithMetricsTB(b)
	defer cleanup()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("concurrent-put-%d-%d", runtime.NumGoroutine(), i)
			value := fmt.Sprintf("concurrent-value-%d", i)
			
			req := createRequest(b, "PUT", fmt.Sprintf("/v1/%s", key), value)
			resp := httptest.NewRecorder()
			server.ServeHTTP(resp, req)
			
			if resp.Code != http.StatusCreated {
				b.Errorf("Concurrent PUT failed with status %d", resp.Code)
			}
			i++
		}
	})
}

func BenchmarkConcurrentGet(b *testing.B) {
	server, cleanup := helpers.CreateTestServerWithMetricsTB(b)
	defer cleanup()

	// Pre-populate
	numKeys := 1000
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("concurrent-get-%d", i)
		value := fmt.Sprintf("concurrent-value-%d", i)
		
		req := createRequest(b, "PUT", fmt.Sprintf("/v1/%s", key), value)
		resp := httptest.NewRecorder()
		server.ServeHTTP(resp, req)
		
		if resp.Code != http.StatusCreated {
			b.Fatalf("Setup failed: PUT returned %d", resp.Code)
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("concurrent-get-%d", i%numKeys)
			
			req := createRequest(b, "GET", fmt.Sprintf("/v1/%s", key), "")
			resp := httptest.NewRecorder()
			server.ServeHTTP(resp, req)
			
			if resp.Code != http.StatusOK {
				b.Errorf("Concurrent GET failed with status %d", resp.Code)
			}
			i++
		}
	})
}

// Performance tests (not benchmarks, but actual tests with performance assertions)
func TestThroughputPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	server, cleanup := helpers.CreateTestServerWithMetrics(t)
	defer cleanup()

	const duration = 5 * time.Second
	const numWorkers = 10

	var totalOps int64
	var mu sync.Mutex
	var wg sync.WaitGroup

	start := time.Now()
	
	// Launch workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			ops := 0
			
			for time.Since(start) < duration {
				key := fmt.Sprintf("throughput-%d-%d", workerID, ops)
				value := fmt.Sprintf("value-%d-%d", workerID, ops)
				
				req := helpers.CreateRequest(t, "PUT", fmt.Sprintf("/v1/%s", key), value)
				resp := httptest.NewRecorder()
				server.ServeHTTP(resp, req)
				
				if resp.Code == http.StatusCreated {
					ops++
				}
			}
			
			mu.Lock()
			totalOps += int64(ops)
			mu.Unlock()
		}(i)
	}
	
	wg.Wait()
	actualDuration := time.Since(start)
	
	throughput := float64(totalOps) / actualDuration.Seconds()
	
	t.Logf("Throughput test: %d operations in %v (%.2f ops/sec)", totalOps, actualDuration, throughput)
	
	// Performance assertions
	assert.Greater(t, throughput, 1000.0, "Should achieve at least 1000 ops/sec")
	assert.Greater(t, totalOps, int64(5000), "Should complete at least 5000 operations in 5 seconds")
}

func TestLatencyPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping latency test in short mode")
	}

	server, cleanup := helpers.CreateTestServerWithMetrics(t)
	defer cleanup()

	const numRequests = 1000
	latencies := make([]time.Duration, numRequests)

	// Warm up
	for i := 0; i < 10; i++ {
		req := helpers.CreateRequest(t, "PUT", fmt.Sprintf("/v1/warmup-%d", i), "warmup-value")
		resp := httptest.NewRecorder()
		server.ServeHTTP(resp, req)
	}

	// Measure latencies
	for i := 0; i < numRequests; i++ {
		key := fmt.Sprintf("latency-test-%d", i)
		value := fmt.Sprintf("latency-value-%d", i)
		
		start := time.Now()
		req := helpers.CreateRequest(t, "PUT", fmt.Sprintf("/v1/%s", key), value)
		resp := httptest.NewRecorder()
		server.ServeHTTP(resp, req)
		latencies[i] = time.Since(start)
		
		require.Equal(t, http.StatusCreated, resp.Code)
	}

	// Calculate statistics
	var total time.Duration
	min := latencies[0]
	max := latencies[0]
	
	for _, lat := range latencies {
		total += lat
		if lat < min {
			min = lat
		}
		if lat > max {
			max = lat
		}
	}
	
	avg := total / time.Duration(numRequests)
	
	// Calculate percentiles (simple approximation)
	// Sort latencies for percentile calculation
	for i := 0; i < len(latencies)-1; i++ {
		for j := i + 1; j < len(latencies); j++ {
			if latencies[i] > latencies[j] {
				latencies[i], latencies[j] = latencies[j], latencies[i]
			}
		}
	}
	
	p50 := latencies[numRequests/2]
	p95 := latencies[int(float64(numRequests)*0.95)]
	p99 := latencies[int(float64(numRequests)*0.99)]
	
	t.Logf("Latency Statistics:")
	t.Logf("  Average: %v", avg)
	t.Logf("  Min: %v", min)
	t.Logf("  Max: %v", max)
	t.Logf("  P50: %v", p50)
	t.Logf("  P95: %v", p95)
	t.Logf("  P99: %v", p99)
	
	// Performance assertions
	assert.Less(t, avg, 10*time.Millisecond, "Average latency should be under 10ms")
	assert.Less(t, p95, 20*time.Millisecond, "P95 latency should be under 20ms")
	assert.Less(t, p99, 50*time.Millisecond, "P99 latency should be under 50ms")
}

func TestMemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory test in short mode")
	}

	server, cleanup := helpers.CreateTestServerWithMetrics(t)
	defer cleanup()

	// Force GC to get baseline
	runtime.GC()
	runtime.GC()
	
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	// Perform operations that should use memory
	const numKeys = 10000
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("memory-test-%d", i)
		value := fmt.Sprintf("memory-value-%d-%s", i, "this-is-a-longer-value-to-use-more-memory")
		
		req := helpers.CreateRequest(t, "PUT", fmt.Sprintf("/v1/%s", key), value)
		resp := httptest.NewRecorder()
		server.ServeHTTP(resp, req)
		
		require.Equal(t, http.StatusCreated, resp.Code)
	}

	// Force GC and measure
	runtime.GC()
	runtime.GC()
	
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	allocatedMB := float64(memAfter.Alloc-memBefore.Alloc) / (1024 * 1024)
	totalAllocMB := float64(memAfter.TotalAlloc-memBefore.TotalAlloc) / (1024 * 1024)
	
	t.Logf("Memory Usage:")
	t.Logf("  Allocated: %.2f MB", allocatedMB)
	t.Logf("  Total Allocated: %.2f MB", totalAllocMB)
	t.Logf("  GC Runs: %d", memAfter.NumGC-memBefore.NumGC)

	// Memory usage assertions (reasonable limits)
	assert.Less(t, allocatedMB, 100.0, "Should not use more than 100MB for 10k keys")
	assert.Less(t, totalAllocMB, 200.0, "Total allocations should be reasonable")
}

func TestStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	server, cleanup := helpers.CreateTestServerWithMetrics(t)
	defer cleanup()

	const duration = 10 * time.Second
	const numWorkers = 20
	const keysPerWorker = 1000

	var wg sync.WaitGroup
	var totalOps int64
	var errors int64
	var mu sync.Mutex

	start := time.Now()

	// Launch stress workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			ops := 0
			errs := 0
			
			for j := 0; j < keysPerWorker && time.Since(start) < duration; j++ {
				key := fmt.Sprintf("stress-%d-%d", workerID, j)
				value := fmt.Sprintf("stress-value-%d-%d", workerID, j)
				
				// PUT
				req := helpers.CreateRequest(t, "PUT", fmt.Sprintf("/v1/%s", key), value)
				resp := httptest.NewRecorder()
				server.ServeHTTP(resp, req)
				ops++
				if resp.Code != http.StatusCreated {
					errs++
				}
				
				// GET
				req = helpers.CreateRequest(t, "GET", fmt.Sprintf("/v1/%s", key), "")
				resp = httptest.NewRecorder()
				server.ServeHTTP(resp, req)
				ops++
				if resp.Code != http.StatusOK {
					errs++
				}
				
				// Occasional DELETE
				if j%10 == 0 {
					req = helpers.CreateRequest(t, "DELETE", fmt.Sprintf("/v1/%s", key), "")
					resp = httptest.NewRecorder()
					server.ServeHTTP(resp, req)
					ops++
					if resp.Code != http.StatusOK {
						errs++
					}
				}
			}
			
			mu.Lock()
			totalOps += int64(ops)
			errors += int64(errs)
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	actualDuration := time.Since(start)

	// Verify server is still responsive
	req := helpers.CreateRequest(t, "GET", "/healthz", "")
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code, "Server should remain responsive after stress test")

	errorRate := float64(errors) / float64(totalOps) * 100
	throughput := float64(totalOps) / actualDuration.Seconds()

	t.Logf("Stress Test Results:")
	t.Logf("  Duration: %v", actualDuration)
	t.Logf("  Total Operations: %d", totalOps)
	t.Logf("  Errors: %d (%.2f%%)", errors, errorRate)
	t.Logf("  Throughput: %.2f ops/sec", throughput)

	// Stress test assertions
	assert.Less(t, errorRate, 1.0, "Error rate should be less than 1%")
	assert.Greater(t, throughput, 500.0, "Should maintain reasonable throughput under stress")
}

func TestConcurrencyStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrency stress test in short mode")
	}

	server, cleanup := helpers.CreateTestServerWithMetrics(t)
	defer cleanup()

	const numGoroutines = 100
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*opsPerGoroutine)

	// Launch many concurrent goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			
			for j := 0; j < opsPerGoroutine; j++ {
				key := fmt.Sprintf("concurrency-%d-%d", goroutineID, j)
				value := fmt.Sprintf("concurrency-value-%d-%d", goroutineID, j)
				
				req := helpers.CreateRequest(t, "PUT", fmt.Sprintf("/v1/%s", key), value)
				resp := httptest.NewRecorder()
				server.ServeHTTP(resp, req)
				
				if resp.Code != http.StatusCreated {
					errors <- fmt.Errorf("PUT failed: goroutine %d, op %d, status %d", goroutineID, j, resp.Code)
					continue
				}
				
				// Verify immediately
				req = helpers.CreateRequest(t, "GET", fmt.Sprintf("/v1/%s", key), "")
				resp = httptest.NewRecorder()
				server.ServeHTTP(resp, req)
				
				if resp.Code != http.StatusOK {
					errors <- fmt.Errorf("GET failed: goroutine %d, op %d, status %d", goroutineID, j, resp.Code)
					continue
				}
				
				if resp.Body.String() != value {
					errors <- fmt.Errorf("Value mismatch: goroutine %d, op %d, expected %s, got %s", goroutineID, j, value, resp.Body.String())
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	errorCount := 0
	for err := range errors {
		t.Log(err)
		errorCount++
	}

	totalOps := numGoroutines * opsPerGoroutine * 2 // PUT + GET
	errorRate := float64(errorCount) / float64(totalOps) * 100

	t.Logf("Concurrency Stress Results:")
	t.Logf("  Goroutines: %d", numGoroutines)
	t.Logf("  Operations per Goroutine: %d", opsPerGoroutine)
	t.Logf("  Total Operations: %d", totalOps)
	t.Logf("  Errors: %d (%.2f%%)", errorCount, errorRate)

	assert.Equal(t, 0, errorCount, "Should have no errors in concurrency stress test")
	assert.Less(t, errorRate, 0.1, "Error rate should be minimal")
}

// Helper function for tests that need testing.TB interface
func createTestRequest(tb testing.TB, method, path, body string) *http.Request {
	req, err := http.NewRequest(method, path, nil)
	if body != "" {
		req, err = http.NewRequest(method, path, strings.NewReader(body))
	}
	if err != nil {
		tb.Fatalf("Failed to create request: %v", err)
	}
	return req
}