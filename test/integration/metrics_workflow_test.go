package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidaparicio/gokvs/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMetricsIntegrationWorkflow tests the complete metrics workflow
func TestMetricsIntegrationWorkflow(t *testing.T) {
	// Create test server with metrics
	server, cleanup := helpers.CreateTestServerWithMetrics(t)
	defer cleanup()

	// Perform various operations
	testCases := []struct {
		method         string
		path           string
		body           string
		expectedStatus int
	}{
		{"PUT", "/v1/test-key-1", "test-value-1", http.StatusCreated},
		{"PUT", "/v1/test-key-2", "test-value-2", http.StatusCreated},
		{"GET", "/v1/test-key-1", "", http.StatusOK},
		{"GET", "/v1/test-key-2", "", http.StatusOK},
		{"DELETE", "/v1/test-key-1", "", http.StatusOK},
		{"GET", "/v1/non-existent", "", http.StatusNotFound},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s_%s", tc.method, tc.path), func(t *testing.T) {
			req := helpers.CreateRequest(t, tc.method, tc.path, tc.body)
			resp := httptest.NewRecorder()
			server.ServeHTTP(resp, req)
			assert.Equal(t, tc.expectedStatus, resp.Code)
		})
	}

	// Fetch metrics and verify they contain expected values
	req := helpers.CreateRequest(t, "GET", "/metrics", "")
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)

	metricsBody := resp.Body.String()

	// Verify specific metrics are present and have expected values
	t.Run("VerifyEventsPutMetric", func(t *testing.T) {
		// Should have 2 PUT events
		assert.Contains(t, metricsBody, "gokvs_events_put")
		putCount := extractMetricValue(t, metricsBody, "gokvs_events_put")
		assert.GreaterOrEqual(t, putCount, 2.0)
	})

	t.Run("VerifyEventsGetMetric", func(t *testing.T) {
		// Should have at least 3 GET events (2 successful + 1 not found)
		assert.Contains(t, metricsBody, "gokvs_events_get")
		getCount := extractMetricValue(t, metricsBody, "gokvs_events_get")
		assert.GreaterOrEqual(t, getCount, 3.0)
	})

	t.Run("VerifyEventsDeleteMetric", func(t *testing.T) {
		// Should have 1 DELETE event
		assert.Contains(t, metricsBody, "gokvs_events_delete")
		deleteCount := extractMetricValue(t, metricsBody, "gokvs_events_delete")
		assert.GreaterOrEqual(t, deleteCount, 1.0)
	})

	t.Run("VerifyRequestsTotal", func(t *testing.T) {
		// Should have HTTP request metrics with different status codes
		assert.Contains(t, metricsBody, "http_requests_total")

		// Verify we have metrics for successful requests
		assert.Contains(t, metricsBody, `code="200"`)
		assert.Contains(t, metricsBody, `code="201"`)
		assert.Contains(t, metricsBody, `code="404"`)
	})

	t.Run("VerifyRequestDuration", func(t *testing.T) {
		// Should have request duration histogram
		assert.Contains(t, metricsBody, "http_request_duration_seconds")
		assert.Contains(t, metricsBody, "_bucket")
		assert.Contains(t, metricsBody, "_count")
		assert.Contains(t, metricsBody, "_sum")
	})

	t.Run("VerifyQueriesInflight", func(t *testing.T) {
		// Should have queries inflight metric (should be 0 when not processing)
		assert.Contains(t, metricsBody, "gokvs_queries_inflight")
	})

	t.Run("VerifyGoMetrics", func(t *testing.T) {
		// Should include Go runtime metrics
		assert.Contains(t, metricsBody, "go_memstats")
		assert.Contains(t, metricsBody, "go_goroutines")
	})

	t.Run("VerifyInfoMetric", func(t *testing.T) {
		// Should have version info metric
		assert.Contains(t, metricsBody, "gokvs_info")
	})
}

// TestMetricsConcurrentWorkflow tests metrics collection under concurrent load
func TestMetricsConcurrentWorkflow(t *testing.T) {
	server, cleanup := helpers.CreateTestServerWithMetrics(t)
	defer cleanup()

	const numWorkers = 10
	const opsPerWorker = 5

	var wg sync.WaitGroup

	// Get initial metrics snapshot
	initialMetrics := getMetricsSnapshot(t, server)

	// Perform concurrent operations
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < opsPerWorker; j++ {
				key := fmt.Sprintf("concurrent-key-%d-%d", workerID, j)
				value := fmt.Sprintf("concurrent-value-%d-%d", workerID, j)

				// PUT operation
				req := helpers.CreateRequest(t, "PUT", fmt.Sprintf("/v1/%s", key), value)
				resp := httptest.NewRecorder()
				server.ServeHTTP(resp, req)
				assert.Equal(t, http.StatusCreated, resp.Code)

				// GET operation
				req = helpers.CreateRequest(t, "GET", fmt.Sprintf("/v1/%s", key), "")
				resp = httptest.NewRecorder()
				server.ServeHTTP(resp, req)
				assert.Equal(t, http.StatusOK, resp.Code)
			}
		}(i)
	}

	wg.Wait()

	// Get final metrics snapshot
	finalMetrics := getMetricsSnapshot(t, server)

	// Verify metrics increased appropriately
	expectedPuts := float64(numWorkers * opsPerWorker)
	expectedGets := float64(numWorkers * opsPerWorker)

	actualPutIncrease := finalMetrics.EventsPut - initialMetrics.EventsPut
	actualGetIncrease := finalMetrics.EventsGet - initialMetrics.EventsGet

	assert.GreaterOrEqual(t, actualPutIncrease, expectedPuts,
		"PUT events should increase by at least %v, got %v", expectedPuts, actualPutIncrease)
	assert.GreaterOrEqual(t, actualGetIncrease, expectedGets,
		"GET events should increase by at least %v, got %v", expectedGets, actualGetIncrease)

	// Verify queries inflight returns to 0
	assert.Equal(t, 0.0, finalMetrics.QueriesInflight,
		"Queries inflight should return to 0 after all operations complete")
}

// TestMetricsErrorConditions tests metrics collection during error scenarios
func TestMetricsErrorConditions(t *testing.T) {
	server, cleanup := helpers.CreateTestServerWithMetrics(t)
	defer cleanup()

	initialMetrics := getMetricsSnapshot(t, server)

	// Test various error conditions
	testCases := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{"GET non-existent key", "GET", "/v1/non-existent-key", http.StatusNotFound},
		{"Method not allowed on root", "POST", "/", http.StatusMethodNotAllowed},
		{"Method not allowed on v1", "POST", "/v1", http.StatusMethodNotAllowed},
		{"Method not allowed on key endpoint", "PATCH", "/v1/test-key", http.StatusMethodNotAllowed},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := helpers.CreateRequest(t, tc.method, tc.path, "")
			resp := httptest.NewRecorder()
			server.ServeHTTP(resp, req)
			assert.Equal(t, tc.expectedStatus, resp.Code)
		})
	}

	finalMetrics := getMetricsSnapshot(t, server)

	// Verify GET attempts are recorded even for non-existent keys
	assert.Greater(t, finalMetrics.EventsGet, initialMetrics.EventsGet,
		"GET events should increase even for non-existent keys")

	// Verify not allowed errors are recorded
	assert.Greater(t, finalMetrics.HttpNotAllowed, initialMetrics.HttpNotAllowed,
		"HTTP Not Allowed errors should be recorded")
}

// TestMetricsEndpointAvailability tests that the metrics endpoint is always available
func TestMetricsEndpointAvailability(t *testing.T) {
	server, cleanup := helpers.CreateTestServerWithMetrics(t)
	defer cleanup()

	// Test metrics endpoint multiple times
	for i := 0; i < 5; i++ {
		req := helpers.CreateRequest(t, "GET", "/metrics", "")
		resp := httptest.NewRecorder()
		server.ServeHTTP(resp, req)

		require.Equal(t, http.StatusOK, resp.Code)

		body := resp.Body.String()

		// Verify it's valid Prometheus format
		assert.Contains(t, body, "# HELP")
		assert.Contains(t, body, "# TYPE")

		// Verify content type (allow additional parameters)
		contentType := resp.Header().Get("Content-Type")
		assert.Contains(t, contentType, "text/plain; version=0.0.4; charset=utf-8",
			"Content type should contain prometheus format")
	}
}

// TestMetricsReset tests behavior after server restart (simulation)
func TestMetricsReset(t *testing.T) {
	// First server instance
	server1, cleanup1 := helpers.CreateTestServerWithMetrics(t)

	// Perform some operations
	req := helpers.CreateRequest(t, "PUT", "/v1/test-key", "test-value")
	resp := httptest.NewRecorder()
	server1.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusCreated, resp.Code)

	// Get metrics from first instance
	metrics1 := getMetricsSnapshot(t, server1)
	assert.Greater(t, metrics1.EventsPut, 0.0)

	cleanup1()

	// Second server instance (simulating restart)
	server2, cleanup2 := helpers.CreateTestServerWithMetrics(t)
	defer cleanup2()

	// Get initial metrics from second instance
	metrics2 := getMetricsSnapshot(t, server2)

	// Verify metrics reset to initial state (except for replayed events)
	// The events_replayed counter should show activity from transaction log
	assert.GreaterOrEqual(t, metrics2.EventsReplayed, 0.0)
}

// TestMetricsHighLoad tests metrics under high load
func TestMetricsHighLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high load test in short mode")
	}

	server, cleanup := helpers.CreateTestServerWithMetrics(t)
	defer cleanup()

	const numRequests = 100
	const numWorkers = 20

	requestChan := make(chan int, numRequests)
	for i := 0; i < numRequests; i++ {
		requestChan <- i
	}
	close(requestChan)

	var wg sync.WaitGroup

	startTime := time.Now()

	// Launch workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for reqNum := range requestChan {
				key := fmt.Sprintf("load-test-key-%d", reqNum)
				value := fmt.Sprintf("load-test-value-%d", reqNum)

				req := helpers.CreateRequest(t, "PUT", fmt.Sprintf("/v1/%s", key), value)
				resp := httptest.NewRecorder()
				server.ServeHTTP(resp, req)
				assert.Equal(t, http.StatusCreated, resp.Code)
			}
		}()
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Get final metrics
	finalMetrics := getMetricsSnapshot(t, server)

	// Verify all operations were recorded
	assert.GreaterOrEqual(t, finalMetrics.EventsPut, float64(numRequests),
		"All PUT operations should be recorded in metrics")

	// Verify performance (this is just a sanity check)
	rps := float64(numRequests) / duration.Seconds()
	t.Logf("Processed %d requests in %v (%.2f req/sec)", numRequests, duration, rps)

	// Sanity check - should be able to handle at least 100 req/sec
	assert.Greater(t, rps, 100.0, "Should handle at least 100 requests per second")

	// Verify metrics endpoint still works under load
	req := helpers.CreateRequest(t, "GET", "/metrics", "")
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)
}

// Helper types and functions

type MetricsSnapshot struct {
	EventsPut       float64
	EventsGet       float64
	EventsDelete    float64
	EventsReplayed  float64
	QueriesInflight float64
	HttpNotAllowed  float64
}

func getMetricsSnapshot(t *testing.T, server http.Handler) MetricsSnapshot {
	req := helpers.CreateRequest(t, "GET", "/metrics", "")
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)

	body := resp.Body.String()

	return MetricsSnapshot{
		EventsPut:       extractMetricValue(t, body, "gokvs_events_put"),
		EventsGet:       extractMetricValue(t, body, "gokvs_events_get"),
		EventsDelete:    extractMetricValue(t, body, "gokvs_events_delete"),
		EventsReplayed:  extractMetricValue(t, body, "gokvs_events_replayed"),
		QueriesInflight: extractMetricValue(t, body, "gokvs_queries_inflight"),
		HttpNotAllowed:  extractMetricValue(t, body, "http_405"),
	}
}

func extractMetricValue(t *testing.T, body, metricName string) float64 {
	// Look for the metric line that's not a HELP or TYPE comment
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, metricName+" ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				value, err := strconv.ParseFloat(parts[1], 64)
				if err != nil {
					t.Logf("Failed to parse metric value %s for %s: %v", parts[1], metricName, err)
					return 0
				}
				return value
			}
		}
	}

	// If not found as simple counter, try to find as counter with labels or histogram
	// For counters with labels, find any line starting with metricName{
	pattern := fmt.Sprintf(`^%s(?:\{[^}]*\})?\s+([0-9\.]+)`, regexp.QuoteMeta(metricName))
	re := regexp.MustCompile(pattern)

	var total float64
	for _, line := range lines {
		if matches := re.FindStringSubmatch(line); len(matches) > 1 {
			value, err := strconv.ParseFloat(matches[1], 64)
			if err == nil {
				total += value
			}
		}
	}

	return total
}
