package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/davidaparicio/gokvs/internal"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// Make these package-level variables to track test state
	testMetrics *internal.Metrics
	testReg     *prometheus.Registry
)

func setupRouter() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/v1/{key}", keyValueGetHandler).Methods("GET")
	r.HandleFunc("/v1/{key}", keyValuePutHandler).Methods("PUT")
	r.HandleFunc("/v1/{key}", keyValueDeleteHandler).Methods("DELETE")
	return r
}

func setupTest(t *testing.T) (*httptest.Server, *internal.TransactionLog, func()) {
	t.Helper()

	// Clear any existing metrics
	if testReg != nil {
		testReg = nil
	}

	// Create a new registry for this test
	testReg = prometheus.NewRegistry()
	testMetrics = internal.NewMetrics(testReg)

	// Set the global metrics variable
	m = testMetrics

	tlog, err := internal.NewTransactionLogger(fmt.Sprintf("/tmp/test-transactions-%d.log", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("Failed to create transaction logger: %v", err)
	}
	tlog.Run()

	transact = tlog // Set the global transaction logger
	//ts := httptest.NewServer(setupRouter())

	// Add healthcheck route to router
	r := setupRouter()
	r.HandleFunc("/healthz", checkMuxHandler)
	ts := httptest.NewServer(r)

	cleanup := func() {
		ts.Close()
		// Wait a brief moment to ensure all operations complete
		time.Sleep(100 * time.Millisecond)
		tlog.Close()
		// Reset global variables for next test
		m = nil
		transact = nil
		testMetrics = nil
		testReg = nil
	}

	return ts, tlog, cleanup
}

func TestKeyValueHandlers(t *testing.T) {
	ts, _, cleanup := setupTest(t)
	defer cleanup()

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
				req, err = http.NewRequest(tt.method, ts.URL+"/v1/"+tt.key, bytes.NewBufferString(tt.value))
			default:
				req, err = http.NewRequest(tt.method, ts.URL+"/v1/"+tt.key, nil)
			}

			if err != nil {
				t.Fatal(err)
			}

			//rr := httptest.NewRecorder()
			//router.ServeHTTP(rr, req)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if status := resp.StatusCode; status != tt.expectedCode {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expectedCode)
			}

			if tt.expectedValue != "" {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				if string(body) != tt.expectedValue {
					t.Errorf("handler returned unexpected body: got %v want %v",
						string(body), tt.expectedValue)
				}
			}
		})
	}
}

func TestHealthCheck(t *testing.T) {
	//router := setupRouter()
	//router.HandleFunc("/healthz", checkMuxHandler)
	ts, _, cleanup := setupTest(t)
	defer cleanup()

	req, err := http.NewRequest("GET", ts.URL+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}

	//rr := httptest.NewRecorder()
	//router.ServeHTTP(rr, req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if status := resp.StatusCode; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	expected := "imok\n"
	if expected != "" {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != expected {
			t.Errorf("handler returned unexpected body: got %v want %v",
				string(body), expected)
		}
	}
}

func TestKeyValueStoreWhitespaceHandling(t *testing.T) {
	// Setup
	//ts := httptest.NewServer(setupRouter())
	//defer ts.Close()
	ts, _, cleanup := setupTest(t)
	defer cleanup()

	tests := []struct {
		name          string
		key           string
		value         string
		expectedCode  int
		expectedValue string
	}{
		{
			name:          "Simple whitespace in value",
			key:           "test1",
			value:         "  hello world  ",
			expectedCode:  http.StatusCreated,
			expectedValue: "  hello world  ",
		},
		{
			name:          "Multi-line value",
			key:           "test2",
			value:         "line1\nline2\nline3",
			expectedCode:  http.StatusCreated,
			expectedValue: "line1\nline2\nline3",
		},
		{
			name:          "Key with whitespace",
			key:           "  test3  ",
			value:         "value",
			expectedCode:  http.StatusCreated,
			expectedValue: "value",
		},
		{
			name:          "Value with tabs and newlines",
			key:           "test4",
			value:         "\tindented\n\tlines\n",
			expectedCode:  http.StatusCreated,
			expectedValue: "\tindented\n\tlines\n",
		},
		{
			name:         "Empty key after trim",
			key:          "   ",
			value:        "value",
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Empty value after trim",
			key:          "test5",
			value:        "   ",
			expectedCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test PUT
			putResp, err := http.Post(
				fmt.Sprintf("%s/v1/%s", ts.URL, tt.key),
				"text/plain",
				strings.NewReader(tt.value),
			)
			if err != nil {
				t.Fatalf("Failed to PUT: %v", err)
			}
			defer putResp.Body.Close()

			if putResp.StatusCode != tt.expectedCode {
				t.Errorf("PUT status code = %v, want %v", putResp.StatusCode, tt.expectedCode)
			}

			// If we expect the PUT to succeed, test GET
			if tt.expectedCode == http.StatusCreated {
				// Test GET
				getResp, err := http.Get(fmt.Sprintf("%s/v1/%s", ts.URL, tt.key))
				if err != nil {
					t.Fatalf("Failed to GET: %v", err)
				}
				defer getResp.Body.Close()

				if getResp.StatusCode != http.StatusOK {
					t.Errorf("GET status code = %v, want %v", getResp.StatusCode, http.StatusOK)
				}

				body, err := io.ReadAll(getResp.Body)
				if err != nil {
					t.Fatalf("Failed to read GET response: %v", err)
				}

				if string(body) != tt.expectedValue {
					t.Errorf("GET value = %q, want %q", string(body), tt.expectedValue)
				}

				// Test DELETE
				deleteReq, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/v1/%s", ts.URL, tt.key), nil)
				if err != nil {
					t.Fatalf("Failed to create DELETE request: %v", err)
				}
				deleteResp, err := http.DefaultClient.Do(deleteReq)
				if err != nil {
					t.Fatalf("Failed to DELETE: %v", err)
				}
				defer deleteResp.Body.Close()

				if deleteResp.StatusCode != http.StatusOK {
					t.Errorf("DELETE status code = %v, want %v", deleteResp.StatusCode, http.StatusOK)
				}

				// Verify key is deleted
				getResp2, err := http.Get(fmt.Sprintf("%s/v1/%s", ts.URL, tt.key))
				if err != nil {
					t.Fatalf("Failed to GET after DELETE: %v", err)
				}
				defer getResp2.Body.Close()

				if getResp2.StatusCode != http.StatusNotFound {
					t.Errorf("GET after DELETE status code = %v, want %v", getResp2.StatusCode, http.StatusNotFound)
				}
			}
		})
	}
}

func TestKeyValueStoreEdgeCases(t *testing.T) {
	ts := httptest.NewServer(setupRouter())
	defer ts.Close()

	tests := []struct {
		name         string
		key          string
		value        string
		operation    string
		expectedCode int
	}{
		{
			name:         "GET with empty key",
			key:          "   ",
			operation:    "GET",
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "DELETE with empty key",
			key:          "   ",
			operation:    "DELETE",
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "PUT with very long multi-line value",
			key:          "longvalue",
			value:        strings.Repeat("line\n", 1000),
			operation:    "PUT",
			expectedCode: http.StatusCreated,
		},
		{
			name:         "PUT with special characters in value",
			key:          "special",
			value:        "∑≈çœ∂¥†®\n\t™¶§",
			operation:    "PUT",
			expectedCode: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp *http.Response
			var err error

			switch tt.operation {
			case "GET":
				resp, err = http.Get(fmt.Sprintf("%s/v1/%s", ts.URL, tt.key))
			case "DELETE":
				req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/v1/%s", ts.URL, tt.key), nil)
				resp, err = http.DefaultClient.Do(req)
			case "PUT":
				resp, err = http.Post(
					fmt.Sprintf("%s/v1/%s", ts.URL, tt.key),
					"text/plain",
					strings.NewReader(tt.value),
				)
			}

			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedCode {
				t.Errorf("%s status code = %v, want %v", tt.operation, resp.StatusCode, tt.expectedCode)
			}

			// For successful PUTs, verify the value can be retrieved correctly
			if tt.operation == "PUT" && tt.expectedCode == http.StatusCreated {
				getResp, err := http.Get(fmt.Sprintf("%s/v1/%s", ts.URL, tt.key))
				if err != nil {
					t.Fatalf("Failed to GET after PUT: %v", err)
				}
				defer getResp.Body.Close()

				body, err := io.ReadAll(getResp.Body)
				if err != nil {
					t.Fatalf("Failed to read GET response: %v", err)
				}

				if string(body) != tt.value {
					t.Errorf("GET value = %q, want %q", string(body), tt.value)
				}
			}
		})
	}
}
