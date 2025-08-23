package helpers

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// CreateRequest creates an http.Request for testing purposes
func CreateRequest(t *testing.T, method, path, body string) *http.Request {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, path, bodyReader)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	return req
}

// HTTPHelper provides utilities for HTTP testing
type HTTPHelper struct {
	t      *testing.T
	client *http.Client
}

// NewHTTPHelper creates a new HTTP helper
func NewHTTPHelper(t *testing.T) *HTTPHelper {
	return &HTTPHelper{
		t: t,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Request represents an HTTP request for testing
type Request struct {
	Method      string
	URL         string
	Body        string
	Headers     map[string]string
	Timeout     time.Duration
	ExpectError bool
}

// Response represents an HTTP response for testing
type Response struct {
	StatusCode int
	Body       string
	Headers    map[string]string
	Duration   time.Duration
}

// SendRequest sends an HTTP request and returns the response
func (hh *HTTPHelper) SendRequest(req Request) (*Response, error) {
	// Set default timeout if not specified
	if req.Timeout == 0 {
		req.Timeout = 5 * time.Second
	}

	// Create HTTP client with timeout
	client := &http.Client{Timeout: req.Timeout}

	// Create request
	var bodyReader io.Reader
	if req.Body != "" {
		bodyReader = strings.NewReader(req.Body)
	}

	httpReq, err := http.NewRequest(req.Method, req.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	// Send request and measure duration
	start := time.Now()
	resp, err := client.Do(httpReq)
	duration := time.Since(start)

	if err != nil {
		if req.ExpectError {
			return &Response{Duration: duration}, nil
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Extract headers
	headers := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       string(bodyBytes),
		Headers:    headers,
		Duration:   duration,
	}, nil
}

// PutKeyValue sends a PUT request to store a key-value pair
func (hh *HTTPHelper) PutKeyValue(baseURL, key, value string) (*Response, error) {
	return hh.SendRequest(Request{
		Method: "PUT",
		URL:    fmt.Sprintf("%s/v1/%s", baseURL, key),
		Body:   value,
		Headers: map[string]string{
			"Content-Type": "text/plain",
		},
	})
}

// GetKeyValue sends a GET request to retrieve a value by key
func (hh *HTTPHelper) GetKeyValue(baseURL, key string) (*Response, error) {
	return hh.SendRequest(Request{
		Method: "GET",
		URL:    fmt.Sprintf("%s/v1/%s", baseURL, key),
	})
}

// DeleteKeyValue sends a DELETE request to remove a key
func (hh *HTTPHelper) DeleteKeyValue(baseURL, key string) (*Response, error) {
	return hh.SendRequest(Request{
		Method: "DELETE",
		URL:    fmt.Sprintf("%s/v1/%s", baseURL, key),
	})
}

// HealthCheck sends a health check request
func (hh *HTTPHelper) HealthCheck(baseURL string) (*Response, error) {
	return hh.SendRequest(Request{
		Method: "GET",
		URL:    fmt.Sprintf("%s/healthz", baseURL),
	})
}

// GetMetrics sends a request to retrieve Prometheus metrics
func (hh *HTTPHelper) GetMetrics(baseURL string) (*Response, error) {
	return hh.SendRequest(Request{
		Method: "GET",
		URL:    fmt.Sprintf("%s/metrics", baseURL),
	})
}

// ConcurrentRequests sends multiple requests concurrently
func (hh *HTTPHelper) ConcurrentRequests(requests []Request) ([]*Response, []error) {
	type result struct {
		response *Response
		err      error
		index    int
	}

	resultChan := make(chan result, len(requests))

	// Send all requests concurrently
	for i, req := range requests {
		go func(index int, request Request) {
			resp, err := hh.SendRequest(request)
			resultChan <- result{
				response: resp,
				err:      err,
				index:    index,
			}
		}(i, req)
	}

	// Collect results
	responses := make([]*Response, len(requests))
	errors := make([]error, len(requests))

	for i := 0; i < len(requests); i++ {
		res := <-resultChan
		responses[res.index] = res.response
		errors[res.index] = res.err
	}

	return responses, errors
}

// LoadTest performs a load test with the specified parameters
func (hh *HTTPHelper) LoadTest(req Request, concurrency, requests int) (*LoadTestResult, error) {
	type requestResult struct {
		response *Response
		err      error
	}

	resultChan := make(chan requestResult, requests)
	semaphore := make(chan struct{}, concurrency)

	start := time.Now()

	// Send requests with concurrency control
	for i := 0; i < requests; i++ {
		go func() {
			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			resp, err := hh.SendRequest(req)
			resultChan <- requestResult{
				response: resp,
				err:      err,
			}
		}()
	}

	// Collect results
	var successful, failed int
	var totalDuration time.Duration
	var minDuration, maxDuration time.Duration
	statusCodes := make(map[int]int)

	for i := 0; i < requests; i++ {
		result := <-resultChan
		if result.err != nil {
			failed++
		} else {
			successful++
			duration := result.response.Duration
			totalDuration += duration

			if minDuration == 0 || duration < minDuration {
				minDuration = duration
			}
			if duration > maxDuration {
				maxDuration = duration
			}

			statusCodes[result.response.StatusCode]++
		}
	}

	totalTestDuration := time.Since(start)

	return &LoadTestResult{
		TotalRequests:      requests,
		SuccessfulRequests: successful,
		FailedRequests:     failed,
		TotalDuration:      totalTestDuration,
		AverageDuration:    totalDuration / time.Duration(successful),
		MinDuration:        minDuration,
		MaxDuration:        maxDuration,
		RequestsPerSecond:  float64(requests) / totalTestDuration.Seconds(),
		StatusCodes:        statusCodes,
	}, nil
}

// LoadTestResult contains the results of a load test
type LoadTestResult struct {
	TotalRequests      int
	SuccessfulRequests int
	FailedRequests     int
	TotalDuration      time.Duration
	AverageDuration    time.Duration
	MinDuration        time.Duration
	MaxDuration        time.Duration
	RequestsPerSecond  float64
	StatusCodes        map[int]int
}

// String returns a string representation of the load test results
func (ltr *LoadTestResult) String() string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "Load Test Results:\n")
	fmt.Fprintf(&buf, "  Total Requests: %d\n", ltr.TotalRequests)
	fmt.Fprintf(&buf, "  Successful: %d\n", ltr.SuccessfulRequests)
	fmt.Fprintf(&buf, "  Failed: %d\n", ltr.FailedRequests)
	fmt.Fprintf(&buf, "  Total Duration: %v\n", ltr.TotalDuration)
	fmt.Fprintf(&buf, "  Average Duration: %v\n", ltr.AverageDuration)
	fmt.Fprintf(&buf, "  Min Duration: %v\n", ltr.MinDuration)
	fmt.Fprintf(&buf, "  Max Duration: %v\n", ltr.MaxDuration)
	fmt.Fprintf(&buf, "  Requests/sec: %.2f\n", ltr.RequestsPerSecond)
	fmt.Fprintf(&buf, "  Status Codes:\n")
	for code, count := range ltr.StatusCodes {
		fmt.Fprintf(&buf, "    %d: %d\n", code, count)
	}

	return buf.String()
}

// AssertResponse asserts that a response matches expected values
func (hh *HTTPHelper) AssertResponse(resp *Response, expectedStatus int, expectedBody string) {
	if resp.StatusCode != expectedStatus {
		hh.t.Errorf("Expected status %d, got %d", expectedStatus, resp.StatusCode)
	}

	if expectedBody != "" && resp.Body != expectedBody {
		hh.t.Errorf("Expected body %q, got %q", expectedBody, resp.Body)
	}
}

// AssertResponseContains asserts that a response body contains the expected string
func (hh *HTTPHelper) AssertResponseContains(resp *Response, expectedStatus int, expectedSubstring string) {
	if resp.StatusCode != expectedStatus {
		hh.t.Errorf("Expected status %d, got %d", expectedStatus, resp.StatusCode)
	}

	if !strings.Contains(resp.Body, expectedSubstring) {
		hh.t.Errorf("Expected body to contain %q, got %q", expectedSubstring, resp.Body)
	}
}
