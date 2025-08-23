package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/davidaparicio/gokvs/internal"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
)

func setupRouter() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/v1/{key}", keyValueGetHandler).Methods("GET")
	r.HandleFunc("/v1/{key}", keyValuePutHandler).Methods("PUT")
	r.HandleFunc("/v1/{key}", keyValueDeleteHandler).Methods("DELETE")
	return r
}

func TestKeyValueHandlers(t *testing.T) {
	// Initialize metrics with a new registry
	reg := prometheus.NewRegistry()
	m = internal.NewMetrics(reg)
	var err error
	transact, err = internal.NewTransactionLogger("/tmp/test-transactions.log")
	if err != nil {
		t.Fatalf("Failed to create transaction logger: %v", err)
	}
	transact.Run()
	defer func() {
		if err := transact.Close(); err != nil {
			t.Errorf("Failed to close transaction logger: %v", err)
		}
	}()

	router := setupRouter()

	tests := []struct {
		name          string
		method        string
		key           string
		value         string
		expectedCode  int
		expectedValue string
	}{
		{
			name:          "Put value",
			method:        "PUT",
			key:           "test-key",
			value:         "test-value",
			expectedCode:  http.StatusCreated,
			expectedValue: "",
		},
		{
			name:          "Get existing value",
			method:        "GET",
			key:           "test-key",
			expectedCode:  http.StatusOK,
			expectedValue: "test-value",
		},
		{
			name:          "Delete value",
			method:        "DELETE",
			key:           "test-key",
			expectedCode:  http.StatusOK,
			expectedValue: "",
		},
		{
			name:          "Get deleted value",
			method:        "GET",
			key:           "test-key",
			expectedCode:  http.StatusNotFound,
			expectedValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			var err error

			switch tt.method {
			case "PUT":
				req, err = http.NewRequest(tt.method, "/v1/"+tt.key, bytes.NewBufferString(tt.value))
			default:
				req, err = http.NewRequest(tt.method, "/v1/"+tt.key, nil)
			}

			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedCode {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expectedCode)
			}

			if tt.expectedValue != "" && rr.Body.String() != tt.expectedValue {
				t.Errorf("handler returned unexpected body: got %v want %v",
					rr.Body.String(), tt.expectedValue)
			}
		})
	}
}

func TestHealthCheck(t *testing.T) {
	router := setupRouter()
	router.HandleFunc("/healthz", checkMuxHandler)

	req, err := http.NewRequest("GET", "/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	expected := "imok\n"
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

// TestRequestValidation tests input validation and sanitization
func TestRequestValidation(t *testing.T) {
	// Initialize metrics with a new registry
	reg := prometheus.NewRegistry()
	m = internal.NewMetrics(reg)
	var err error
	transact, err = internal.NewTransactionLogger("/tmp/test-validation-transactions.log")
	if err != nil {
		t.Fatalf("Failed to create transaction logger: %v", err)
	}
	transact.Run()
	defer func() {
		if err := transact.Close(); err != nil {
			t.Errorf("Failed to close transaction logger: %v", err)
		}
		os.Remove("/tmp/test-validation-transactions.log")
	}()

	router := setupRouter()

	tests := []struct {
		name          string
		method        string
		key           string
		value         string
		expectedCode  int
		description   string
	}{
		{
			name:          "Very long key",
			method:        "PUT",
			key:           strings.Repeat("k", 10000),
			value:         "test-value",
			expectedCode:  http.StatusCreated,
			description:   "Very long key should be accepted",
		},
		{
			name:          "Special characters in key",
			method:        "PUT",
			key:           "key_with-special.chars_123",
			value:         "test-value",
			expectedCode:  http.StatusCreated,
			description:   "URL-safe special characters in key should be accepted",
		},
		{
			name:          "UTF-8 key",
			method:        "PUT",
			key:           "测试键",
			value:         "测试值",
			expectedCode:  http.StatusCreated,
			description:   "UTF-8 characters should be accepted",
		},
		{
			name:          "Empty value",
			method:        "PUT",
			key:           "empty-value-key",
			value:         "",
			expectedCode:  http.StatusCreated,
			description:   "Empty value should be accepted",
		},
		{
			name:          "Binary data in value",
			method:        "PUT",
			key:           "binary-key",
			value:         "\x00\x01\x02\x03\xFF",
			expectedCode:  http.StatusCreated,
			description:   "Binary data should be accepted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			var err error

			switch tt.method {
			case "PUT":
				req, err = http.NewRequest(tt.method, "/v1/"+tt.key, bytes.NewBufferString(tt.value))
			default:
				req, err = http.NewRequest(tt.method, "/v1/"+tt.key, nil)
			}

			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedCode {
				t.Errorf("%s - %s: expected status %d, got %d", tt.name, tt.description, tt.expectedCode, status)
			}
		})
	}
}

// TestConcurrentRequests tests multiple simultaneous requests
func TestConcurrentRequests(t *testing.T) {
	// Initialize metrics with a new registry
	reg := prometheus.NewRegistry()
	m = internal.NewMetrics(reg)
	var err error
	transact, err = internal.NewTransactionLogger("/tmp/test-concurrent-transactions.log")
	if err != nil {
		t.Fatalf("Failed to create transaction logger: %v", err)
	}
	transact.Run()
	defer func() {
		if err := transact.Close(); err != nil {
			t.Errorf("Failed to close transaction logger: %v", err)
		}
		os.Remove("/tmp/test-concurrent-transactions.log")
	}()

	router := setupRouter()

	const (
		numGoroutines = 20
		numRequests   = 10
	)

	var wg sync.WaitGroup
	errorChan := make(chan error, numGoroutines*numRequests)

	// Test concurrent PUT requests
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numRequests; j++ {
				key := fmt.Sprintf("concurrent_key_%d_%d", id, j)
				value := fmt.Sprintf("concurrent_value_%d_%d", id, j)

				req, err := http.NewRequest("PUT", "/v1/"+key, bytes.NewBufferString(value))
				if err != nil {
					errorChan <- fmt.Errorf("Failed to create request: %w", err)
					return
				}

				rr := httptest.NewRecorder()
				router.ServeHTTP(rr, req)

				if status := rr.Code; status != http.StatusCreated {
					errorChan <- fmt.Errorf("PUT %s: expected status %d, got %d", key, http.StatusCreated, status)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errorChan)

	// Check for errors
	for err := range errorChan {
		t.Error(err)
	}
}
