package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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
