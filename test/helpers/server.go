package helpers

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/davidaparicio/gokvs/internal"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	if !rw.written {
		rw.statusCode = http.StatusOK
		rw.written = true
	}
	return rw.ResponseWriter.Write(data)
}

// TestServer encapsulates a test server instance with all dependencies
type TestServer struct {
	Server    *httptest.Server
	Router    *mux.Router
	Metrics   *internal.Metrics
	Logger    internal.TransactionLogger
	Registry  *prometheus.Registry
	TempFiles []string
	t         *testing.T
}

// NewTestServer creates a new test server with all dependencies initialized
func NewTestServer(t *testing.T) *TestServer {
	ts := &TestServer{
		t:         t,
		TempFiles: make([]string, 0),
	}

	// Create a new metrics registry for isolation
	ts.Registry = prometheus.NewRegistry()
	ts.Metrics = internal.NewMetrics(ts.Registry)

	// Create temporary transaction log file
	logFile, err := os.CreateTemp("", "test-transaction-*.log")
	if err != nil {
		t.Fatalf("Failed to create temporary log file: %v", err)
	}
	ts.TempFiles = append(ts.TempFiles, logFile.Name())
	logFile.Close()

	// Initialize transaction logger
	ts.Logger, err = internal.NewTransactionLogger(logFile.Name())
	if err != nil {
		t.Fatalf("Failed to create transaction logger: %v", err)
	}

	// Start the transaction logger
	ts.Logger.Run()

	// Create router with test handlers
	ts.Router = ts.createRouter()

	// Create test server
	ts.Server = httptest.NewServer(ts.Router)

	return ts
}

// createRouter creates a router with the standard handlers
func (ts *TestServer) createRouter() *mux.Router {
	r := mux.NewRouter()

	// Health check handler
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("imok\n"))
	}).Methods("GET")

	// Metrics endpoint
	r.Handle("/metrics", promhttp.HandlerFor(ts.Registry, promhttp.HandlerOpts{}))

	// KV handlers would be added here in actual implementation
	// For now, we'll create simple handlers for testing
	r.HandleFunc("/v1/{key}", ts.getHandler).Methods("GET")
	r.HandleFunc("/v1/{key}", ts.putHandler).Methods("PUT")
	r.HandleFunc("/v1/{key}", ts.deleteHandler).Methods("DELETE")

	return r
}

// Simple handlers for testing (these would normally be in cmd/server)
func (ts *TestServer) getHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	ts.Metrics.EventsGet.Inc()

	value, err := internal.Get(key)
	if err != nil {
		if err == internal.ErrorNoSuchKey {
			ts.Metrics.HttpNotAllowed.Inc()
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(value))
}

func (ts *TestServer) putHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	ts.Metrics.EventsPut.Inc()

	// Read value from request body
	value := make([]byte, r.ContentLength)
	r.Body.Read(value)

	err := internal.Put(key, string(value))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Log the transaction
	ts.Logger.WritePut(key, string(value))

	w.WriteHeader(http.StatusCreated)
}

func (ts *TestServer) deleteHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	ts.Metrics.EventsDelete.Inc()

	err := internal.Delete(key)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Log the transaction
	ts.Logger.WriteDelete(key)

	w.WriteHeader(http.StatusOK)
}

// Close cleans up the test server and all associated resources
func (ts *TestServer) Close() {
	if ts.Server != nil {
		ts.Server.Close()
	}

	if ts.Logger != nil {
		if err := ts.Logger.Close(); err != nil {
			ts.t.Logf("Failed to close transaction logger: %v", err)
		}
	}

	// Clean up temporary files
	for _, file := range ts.TempFiles {
		if err := os.Remove(file); err != nil {
			ts.t.Logf("Failed to remove temporary file %s: %v", file, err)
		}
	}
}

// URL returns the base URL of the test server
func (ts *TestServer) URL() string {
	return ts.Server.URL
}

// WaitForLogger waits for the transaction logger to process all pending writes
func (ts *TestServer) WaitForLogger() {
	ts.Logger.Wait()
	// Add a small delay to ensure all operations complete
	time.Sleep(10 * time.Millisecond)
}

// GetMetricsValue retrieves a metric value for testing
func (ts *TestServer) GetMetricsValue(metricName string) (float64, error) {
	metrics, err := ts.Registry.Gather()
	if err != nil {
		return 0, fmt.Errorf("failed to gather metrics: %w", err)
	}

	for _, family := range metrics {
		if family.GetName() == metricName {
			if len(family.GetMetric()) > 0 {
				metric := family.GetMetric()[0]
				if metric.GetCounter() != nil {
					return metric.GetCounter().GetValue(), nil
				}
				if metric.GetGauge() != nil {
					return metric.GetGauge().GetValue(), nil
				}
			}
		}
	}

	return 0, fmt.Errorf("metric %s not found", metricName)
}

// CreateTestServerWithMetrics creates a test server configured like the actual server
// Returns the router (for direct testing) and a cleanup function
func CreateTestServerWithMetrics(t *testing.T) (http.Handler, func()) {
	return createTestServerWithMetricsImpl(t)
}

// CreateTestServerWithMetricsTB creates a test server configured like the actual server
// for testing.TB interface (works with both *testing.T and *testing.B)
// Returns the router (for direct testing) and a cleanup function
func CreateTestServerWithMetricsTB(tb testing.TB) (http.Handler, func()) {
	return createTestServerWithMetricsImpl(tb)
}

// createTestServerWithMetricsImpl is the actual implementation that works with testing.TB
func createTestServerWithMetricsImpl(tb testing.TB) (http.Handler, func()) {
	// Create a non-global registry like the actual server
	reg := prometheus.NewRegistry()

	// Keep all the golang default metrics like the actual server
	reg.MustRegister(prometheus.NewGoCollector())

	// Create metrics
	m := internal.NewMetrics(reg)

	// Set info metric like the actual server
	m.Info.With(prometheus.Labels{"version": "test"}).Set(1)

	// Create temporary transaction log files
	tempDir := tb.TempDir() // This automatically cleans up
	logFile := tempDir + "/transactions.log"
	dbFile := tempDir + "/transactions.db"

	// Initialize transaction logger with config similar to actual server
	config := internal.LoggerConfig{
		Type:            "sqlite",
		FilePath:        logFile,
		DBPath:          dbFile,
		MigrateFromFile: true,
	}

	transact, err := internal.NewTransactionLoggerWithConfig(config)
	if err != nil {
		tb.Fatalf("Failed to create transaction logger: %v", err)
	}

	// Start the transaction logger
	transact.Run()

	// Create router with handlers that match the actual server
	r := mux.NewRouter()

	// Add prometheus middleware like the actual server
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			timer := prometheus.NewTimer(m.RequestDurationHistogram.WithLabelValues(r.Method, r.RequestURI))
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK} // Default to 200
			next.ServeHTTP(wrapped, r)
			m.RequestsTotal.WithLabelValues(fmt.Sprintf("%d", wrapped.statusCode), r.Method).Inc()
			timer.ObserveDuration()
		})
	})

	// Not allowed handler
	notAllowedHandler := func(w http.ResponseWriter, r *http.Request) {
		m.HttpNotAllowed.Inc()
		http.Error(w, "Not Allowed", http.StatusMethodNotAllowed)
	}

	// Key-value handlers matching the actual server
	keyValuePutHandler := func(w http.ResponseWriter, r *http.Request) {
		m.QueriesInflight.Inc()
		defer m.QueriesInflight.Dec()
		vars := mux.Vars(r)
		key := vars["key"]

		// Read request body
		value, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		err = internal.Put(key, string(value))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		transact.WritePut(key, string(value))
		m.EventsPut.Inc()
	}

	keyValueGetHandler := func(w http.ResponseWriter, r *http.Request) {
		m.QueriesInflight.Inc()
		defer m.QueriesInflight.Dec()
		vars := mux.Vars(r)
		key := vars["key"]

		value, err := internal.Get(key)
		if err == internal.ErrorNoSuchKey {
			http.Error(w, err.Error(), http.StatusNotFound)
			m.EventsGet.Inc() // Still count the GET attempt
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write([]byte(value))
		m.EventsGet.Inc()
	}

	keyValueDeleteHandler := func(w http.ResponseWriter, r *http.Request) {
		m.QueriesInflight.Inc()
		defer m.QueriesInflight.Dec()
		vars := mux.Vars(r)
		key := vars["key"]

		err := internal.Delete(key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		transact.WriteDelete(key)
		m.EventsDelete.Inc()
	}

	checkHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("imok\n"))
	}

	// Associate paths with handlers exactly like the actual server
	r.HandleFunc("/v1/{key}", keyValueGetHandler).Methods("GET")
	r.HandleFunc("/v1/{key}", keyValuePutHandler).Methods("PUT")
	r.HandleFunc("/v1/{key}", keyValueDeleteHandler).Methods("DELETE")

	r.HandleFunc("/healthz", checkHandler)
	r.HandleFunc("/ruok", checkHandler)

	// Expose metrics endpoint
	r.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))

	// Default handlers for unmatched routes
	r.HandleFunc("/", notAllowedHandler)
	r.HandleFunc("/v1", notAllowedHandler)
	r.HandleFunc("/v1/{key}", notAllowedHandler) // This will catch other methods

	// Cleanup function
	cleanup := func() {
		if err := transact.Close(); err != nil {
			tb.Logf("Failed to close transaction logger: %v", err)
		}
		// tempDir automatically cleaned up by tb.TempDir()
	}

	return r, cleanup
}

// CreateTempFile creates a temporary file and tracks it for cleanup
func (ts *TestServer) CreateTempFile(pattern string) (*os.File, error) {
	tmpFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return nil, err
	}
	ts.TempFiles = append(ts.TempFiles, tmpFile.Name())
	return tmpFile, nil
}
