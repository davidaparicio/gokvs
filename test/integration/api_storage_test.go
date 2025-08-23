package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/davidaparicio/gokvs/test/helpers"
)

// TestAPIStorageIntegration tests the interaction between HTTP API and storage layers
func TestAPIStorageIntegration(t *testing.T) {
	// Create test server with all components
	testServer := helpers.NewTestServer(t)
	defer testServer.Close()

	httpHelper := helpers.NewHTTPHelper(t)
	assert := helpers.NewAssertions(t)

	// Test full CRUD workflow
	t.Run("Complete CRUD workflow", func(t *testing.T) {
		key := "integration_test_key"
		value := "integration_test_value"

		// PUT operation
		putResp, err := httpHelper.PutKeyValue(testServer.URL(), key, value)
		assert.NoError(err, "PUT request should succeed")
		assert.HTTPStatusCode(putResp, http.StatusCreated, "PUT should return 201")

		// Verify data was persisted
		testServer.WaitForLogger()

		// GET operation
		getResp, err := httpHelper.GetKeyValue(testServer.URL(), key)
		assert.NoError(err, "GET request should succeed")
		assert.HTTPStatusCode(getResp, http.StatusOK, "GET should return 200")
		assert.HTTPBodyContains(getResp, value, "GET should return stored value")

		// DELETE operation
		delResp, err := httpHelper.DeleteKeyValue(testServer.URL(), key)
		assert.NoError(err, "DELETE request should succeed")
		assert.HTTPStatusCode(delResp, http.StatusOK, "DELETE should return 200")

		// Verify deletion
		getResp2, err := httpHelper.GetKeyValue(testServer.URL(), key)
		assert.NoError(err, "GET request after delete should succeed")
		assert.HTTPStatusCode(getResp2, http.StatusNotFound, "GET after delete should return 404")
	})

	// Test transaction logging during HTTP operations
	t.Run("Transaction logging during operations", func(t *testing.T) {
		key := "txn_test_key"
		value := "txn_test_value"

		// Perform operations
		putResp, err := httpHelper.PutKeyValue(testServer.URL(), key, value)
		assert.NoError(err, "PUT request should succeed")
		assert.HTTPStatusCode(putResp, http.StatusCreated, "PUT should return 201")

		testServer.WaitForLogger()

		delResp, err := httpHelper.DeleteKeyValue(testServer.URL(), key)
		assert.NoError(err, "DELETE request should succeed")
		assert.HTTPStatusCode(delResp, http.StatusOK, "DELETE should return 200")

		testServer.WaitForLogger()

		// Create new server instance to test recovery
		newTestServer := helpers.NewTestServer(t)
		defer newTestServer.Close()

		// The data should have been logged and can be replayed
		// Since we deleted the key, it should not exist after replay
		getResp, err := httpHelper.GetKeyValue(newTestServer.URL(), key)
		assert.NoError(err, "GET request should succeed")
		assert.HTTPStatusCode(getResp, http.StatusNotFound, "Key should not exist after replay")
	})

	// Test error propagation between layers
	t.Run("Error propagation", func(t *testing.T) {
		// Test GET non-existent key
		getResp, err := httpHelper.GetKeyValue(testServer.URL(), "non_existent_key")
		assert.NoError(err, "GET request should not error at HTTP level")
		assert.HTTPStatusCode(getResp, http.StatusNotFound, "Non-existent key should return 404")
		assert.HTTPBodyContains(getResp, "no such key", "Should return appropriate error message")

		// Test malformed requests
		req := helpers.Request{
			Method: "PATCH",
			URL:    testServer.URL() + "/v1/test_key",
			Body:   "test_value",
		}
		resp, err := httpHelper.SendRequest(req)
		assert.NoError(err, "Request should be sent")
		assert.HTTPStatusCode(resp, http.StatusMethodNotAllowed, "PATCH should not be allowed")
	})

	// Test concurrent operations across layers
	t.Run("Concurrent operations", func(t *testing.T) {
		const numOperations = 50
		requests := make([]helpers.Request, numOperations*3) // PUT, GET, DELETE

		// Create concurrent requests
		for i := 0; i < numOperations; i++ {
			key := fmt.Sprintf("concurrent_key_%d", i)
			value := fmt.Sprintf("concurrent_value_%d", i)

			// PUT request
			requests[i*3] = helpers.Request{
				Method: "PUT",
				URL:    testServer.URL() + "/v1/" + key,
				Body:   value,
			}

			// GET request
			requests[i*3+1] = helpers.Request{
				Method: "GET",
				URL:    testServer.URL() + "/v1/" + key,
			}

			// DELETE request
			requests[i*3+2] = helpers.Request{
				Method: "DELETE",
				URL:    testServer.URL() + "/v1/" + key,
			}
		}

		// Execute requests concurrently
		responses, errors := httpHelper.ConcurrentRequests(requests)

		// Verify all requests completed
		assert.Equal(len(requests), len(responses), "All requests should have responses")

		// Count successes
		successCount := 0
		for i, err := range errors {
			if err == nil && responses[i] != nil {
				successCount++
			}
		}

		// Should have high success rate
		assert.Greater(successCount, numOperations*2, "Should have high success rate")
	})
}

// TestStateConsistency tests state consistency during failures
func TestStateConsistency(t *testing.T) {
	testServer := helpers.NewTestServer(t)
	defer testServer.Close()

	httpHelper := helpers.NewHTTPHelper(t)
	assert := helpers.NewAssertions(t)

	// Test recovery after server restart
	t.Run("Recovery after restart", func(t *testing.T) {
		key := "persistent_key"
		value := "persistent_value"

		// Store data
		putResp, err := httpHelper.PutKeyValue(testServer.URL(), key, value)
		assert.NoError(err, "PUT should succeed")
		assert.HTTPStatusCode(putResp, http.StatusCreated, "PUT should return 201")

		testServer.WaitForLogger()

		// Create new server instance (simulating restart)
		newTestServer := helpers.NewTestServer(t)
		defer newTestServer.Close()

		// Data should be recovered
		getResp, err := httpHelper.GetKeyValue(newTestServer.URL(), key)
		assert.NoError(err, "GET should succeed")
		assert.HTTPStatusCode(getResp, http.StatusOK, "Key should exist after recovery")
		assert.HTTPBodyContains(getResp, value, "Value should be recovered correctly")
	})

	// Test partial operation handling
	t.Run("Partial operation handling", func(t *testing.T) {
		key := "partial_key"
		value := "partial_value"

		// Store initial data
		putResp, err := httpHelper.PutKeyValue(testServer.URL(), key, value)
		assert.NoError(err, "PUT should succeed")
		assert.HTTPStatusCode(putResp, http.StatusCreated, "PUT should return 201")

		testServer.WaitForLogger()

		// Verify the data exists
		getResp, err := httpHelper.GetKeyValue(testServer.URL(), key)
		assert.NoError(err, "GET should succeed")
		assert.HTTPStatusCode(getResp, http.StatusOK, "Key should exist")
		assert.HTTPBodyContains(getResp, value, "Value should match")

		// Test update
		newValue := "updated_partial_value"
		putResp2, err := httpHelper.PutKeyValue(testServer.URL(), key, newValue)
		assert.NoError(err, "PUT update should succeed")
		assert.HTTPStatusCode(putResp2, http.StatusCreated, "PUT update should return 201")

		testServer.WaitForLogger()

		// Verify update
		getResp2, err := httpHelper.GetKeyValue(testServer.URL(), key)
		assert.NoError(err, "GET after update should succeed")
		assert.HTTPStatusCode(getResp2, http.StatusOK, "Key should still exist")
		assert.HTTPBodyContains(getResp2, newValue, "Value should be updated")
	})
}

// TestLargeDataHandling tests handling of large payloads across layers
func TestLargeDataHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large data test in short mode")
	}

	testServer := helpers.NewTestServer(t)
	defer testServer.Close()

	httpHelper := helpers.NewHTTPHelper(t)
	assert := helpers.NewAssertions(t)

	t.Run("Large value storage", func(t *testing.T) {
		key := "large_data_key"
		// Create 1MB value
		largeValue := strings.Repeat("x", 1024*1024)

		// Store large value
		putResp, err := httpHelper.PutKeyValue(testServer.URL(), key, largeValue)
		assert.NoError(err, "PUT large value should succeed")
		assert.HTTPStatusCode(putResp, http.StatusCreated, "PUT should return 201")

		testServer.WaitForLogger()

		// Retrieve large value
		getResp, err := httpHelper.GetKeyValue(testServer.URL(), key)
		assert.NoError(err, "GET large value should succeed")
		assert.HTTPStatusCode(getResp, http.StatusOK, "GET should return 200")
		assert.Equal(len(largeValue), len(getResp.Body), "Retrieved value should have same length")
		assert.Equal(largeValue, getResp.Body, "Retrieved value should match stored value")

		// Clean up
		delResp, err := httpHelper.DeleteKeyValue(testServer.URL(), key)
		assert.NoError(err, "DELETE should succeed")
		assert.HTTPStatusCode(delResp, http.StatusOK, "DELETE should return 200")
	})
}

// TestPerformanceIntegration tests performance across components
func TestPerformanceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	testServer := helpers.NewTestServer(t)
	defer testServer.Close()

	httpHelper := helpers.NewHTTPHelper(t)
	assert := helpers.NewAssertions(t)

	t.Run("Load test", func(t *testing.T) {
		// Perform load test
		req := helpers.Request{
			Method: "PUT",
			URL:    testServer.URL() + "/v1/load_test_key",
			Body:   "load_test_value",
		}

		result, err := httpHelper.LoadTest(req, 10, 100) // 10 concurrent, 100 requests
		assert.NoError(err, "Load test should complete")

		// Verify results
		assert.Greater(result.SuccessfulRequests, 90, "Should have high success rate")
		assert.Less(result.FailedRequests, 10, "Should have low failure rate")
		assert.Greater(result.RequestsPerSecond, 10.0, "Should achieve reasonable throughput")

		t.Logf("Load test results: %s", result.String())
	})
}
