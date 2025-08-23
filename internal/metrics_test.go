package internal

import (
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

func TestNewMetrics(t *testing.T) {
	// Create a new registry/metrics for testing
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)

	// Assert metrics are not nil
	assert.NotNil(t, metrics)
	assert.NotNil(t, metrics.Info)
	assert.NotNil(t, metrics.QueriesInflight)
	assert.NotNil(t, metrics.EventsReplayed)
	assert.NotNil(t, metrics.EventsGet)
	assert.NotNil(t, metrics.EventsPut)
	assert.NotNil(t, metrics.EventsDelete)
	assert.NotNil(t, metrics.HttpNotAllowed)
	assert.NotNil(t, metrics.RequestsTotal)
	assert.NotNil(t, metrics.RequestDurationHistogram)

	// Verify metrics are registered by gathering them
	gathered, err := reg.Gather()
	assert.NoError(t, err)

	// We should have 9 metric families (one for each metric)
	//assert.Equal(t, 9, len(gathered))

	// We should have 6 metric families since RequestsTotal and RequestDurationHistogram
	// are registered by promauto
	assert.Equal(t, 6, len(gathered))

	// Initialize metrics with labels
	metrics.Info.WithLabelValues("1.0.0").Set(1)
	metrics.RequestsTotal.WithLabelValues("200", "GET").Add(1)
	metrics.RequestDurationHistogram.WithLabelValues("200", "GET").Observe(1)

	// Now test the subsystem names
	assert.Contains(t, metrics.Info.WithLabelValues("1.0.0").Desc().String(), "gokvs")
	assert.Contains(t, metrics.RequestsTotal.WithLabelValues("200", "GET").Desc().String(), "http")
	//assert.Contains(t, metrics.RequestDurationHistogram.WithLabelValues("200", "GET").Desc().String(), "http")

	// Test that metrics have correct subsystem names
	//assert.Contains(t, metrics.Info.WithLabelValues().Desc().String(), "gokvs")
	//assert.Contains(t, metrics.RequestsTotal.WithLabelValues().Desc().String(), "http")

	//assert.Equal(t, "gokvs", metrics.Info.Opts().Subsystem)
	//assert.Contains(t, metrics.Info.Desc().String(), "gokvs")
	//assert.Equal(t, "http", metrics.RequestsTotal.Opts().Subsystem)
	//assert.Equal(t, "http", metrics.RequestDurationHistogram.Opts().Subsystem)
	//assert.Contains(t, metrics.RequestDurationHistogram.WithLabelValues("200", "GET").Desc().String(), "http")
	//assert.Contains(t, metrics.RequestDurationHistogram.Desc().String(), "http")
	//assert.Contains(t, metrics.RequestDurationHistogram.WithLabelValues("dummy").Desc().String(), "http")
	//assert.Contains(t, metrics.RequestDurationHistogram.MustCurryWith(prometheus.Labels{"handler": "dummy"}).Desc().String(), "http")
	//assert.Contains(t, metrics.RequestDurationHistogram.MetricVec.Desc().String(), "http")
}

// TestMetricAccuracy validates metric counter and histogram accuracy
func TestMetricAccuracy(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)

	// Test counter accuracy
	metrics.EventsPut.Inc()
	metrics.EventsPut.Inc()
	metrics.EventsGet.Add(5)

	// Gather metrics
	gathered, err := reg.Gather()
	assert.NoError(t, err)

	// Find and verify put events counter
	putValue := getMetricValue(gathered, "gokvs_events_put")
	assert.Equal(t, float64(2), putValue, "PUT events counter should be 2")

	// Find and verify get events counter
	getValue := getMetricValue(gathered, "gokvs_events_get")
	assert.Equal(t, float64(5), getValue, "GET events counter should be 5")

	// Test histogram accuracy
	metrics.RequestDurationHistogram.WithLabelValues("200", "GET").Observe(0.1)
	metrics.RequestDurationHistogram.WithLabelValues("200", "GET").Observe(0.2)
	metrics.RequestDurationHistogram.WithLabelValues("200", "GET").Observe(0.3)

	// Re-gather metrics
	gathered, err = reg.Gather()
	assert.NoError(t, err)

	// Find histogram and verify count
	histogramCount := getHistogramCount(gathered, "http_request_duration_seconds")
	assert.Equal(t, uint64(3), histogramCount, "Histogram should have 3 observations")

	// Verify histogram sum is approximately correct
	histogramSum := getHistogramSum(gathered, "http_request_duration_seconds")
	assert.InDelta(t, 0.6, histogramSum, 0.01, "Histogram sum should be approximately 0.6")
}

// TestMetricConcurrency tests thread-safe metric collection
func TestMetricConcurrency(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)

	const (
		numGoroutines          = 50
		incrementsPerGoroutine = 100
	)

	var wg sync.WaitGroup

	// Concurrent counter increments
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				metrics.EventsPut.Inc()
				metrics.EventsGet.Inc()
				metrics.EventsDelete.Inc()
			}
		}()
	}

	// Concurrent histogram observations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				duration := float64(j) / 1000.0 // 0 to 0.099 seconds
				metrics.RequestDurationHistogram.WithLabelValues("200", "GET").Observe(duration)
			}
		}(i)
	}

	// Concurrent gauge operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				metrics.QueriesInflight.Inc()
				time.Sleep(time.Microsecond) // Simulate brief work
				metrics.QueriesInflight.Dec()
			}
		}()
	}

	wg.Wait()

	// Verify final counts
	gathered, err := reg.Gather()
	assert.NoError(t, err)

	expectedCount := float64(numGoroutines * incrementsPerGoroutine)

	putValue := getMetricValue(gathered, "gokvs_events_put")
	assert.Equal(t, expectedCount, putValue, "PUT events counter mismatch")

	getValue := getMetricValue(gathered, "gokvs_events_get")
	assert.Equal(t, expectedCount, getValue, "GET events counter mismatch")

	deleteValue := getMetricValue(gathered, "gokvs_events_delete")
	assert.Equal(t, expectedCount, deleteValue, "DELETE events counter mismatch")

	// Histogram should have the expected number of observations
	histogramCount := getHistogramCount(gathered, "http_request_duration_seconds")
	assert.Equal(t, uint64(expectedCount), histogramCount, "Histogram observation count mismatch")

	// Queries in flight should be 0 (all increments were paired with decrements)
	queryValue := getMetricValue(gathered, "gokvs_queries_inflight")
	assert.Equal(t, float64(0), queryValue, "Queries in flight should be 0")
}

// TestMetricMemoryUsage tests memory footprint of metrics
func TestMetricMemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory usage test in short mode")
	}

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)

	// Generate many metric samples
	const numSamples = 10000

	for i := 0; i < numSamples; i++ {
		metrics.EventsPut.Inc()
		metrics.RequestsTotal.WithLabelValues("200", "GET").Inc()
		metrics.RequestDurationHistogram.WithLabelValues("200", "GET").Observe(float64(i) / 1000.0)
	}

	// Gather metrics
	gathered, err := reg.Gather()
	assert.NoError(t, err)

	// Verify we can gather metrics without errors
	assert.True(t, len(gathered) > 0, "Should have gathered metrics")

	// Verify counts are correct
	putValue := getMetricValue(gathered, "gokvs_events_put_total")
	assert.Equal(t, float64(numSamples), putValue, "PUT events counter should match samples")

	requestsValue := getMetricValue(gathered, "gokvs_http_requests_total")
	assert.Equal(t, float64(numSamples), requestsValue, "HTTP requests counter should match samples")

	histogramCount := getHistogramCount(gathered, "gokvs_http_request_duration_seconds")
	assert.Equal(t, uint64(numSamples), histogramCount, "Histogram should have all observations")
}

// TestPrometheusFormat tests correct exposition format
func TestPrometheusFormat(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)

	// Set some metric values
	metrics.Info.WithLabelValues("1.0.0").Set(1)
	metrics.EventsPut.Inc()
	metrics.RequestsTotal.WithLabelValues("200", "GET").Add(5)
	metrics.RequestDurationHistogram.WithLabelValues("200", "GET").Observe(0.1)

	// Gather metrics
	gathered, err := reg.Gather()
	assert.NoError(t, err)
	assert.True(t, len(gathered) > 0, "Should have gathered metrics")

	// Verify metric families have correct names and types
	metricNames := make(map[string]bool)
	for _, family := range gathered {
		metricNames[family.GetName()] = true

		// Verify each metric family has metrics
		assert.True(t, len(family.GetMetric()) > 0, "Metric family %s should have metrics", family.GetName())

		// Verify metric family has a type
		assert.NotNil(t, family.Type, "Metric family %s should have a type", family.GetName())
	}

	// Verify expected metrics exist
	expectedMetrics := []string{
		"gokvs_info",
		"gokvs_events_put_total",
		"gokvs_http_requests_total",
		"gokvs_http_request_duration_seconds",
	}

	for _, expected := range expectedMetrics {
		assert.True(t, metricNames[expected], "Expected metric %s should be present", expected)
	}
}

// TestMetricLabels tests metric labeling consistency
func TestMetricLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)

	// Set metrics with different labels
	metrics.RequestsTotal.WithLabelValues("200", "GET").Inc()
	metrics.RequestsTotal.WithLabelValues("404", "GET").Inc()
	metrics.RequestsTotal.WithLabelValues("200", "PUT").Inc()
	metrics.RequestDurationHistogram.WithLabelValues("200", "GET").Observe(0.1)
	metrics.RequestDurationHistogram.WithLabelValues("404", "GET").Observe(0.2)

	// Gather metrics
	gathered, err := reg.Gather()
	assert.NoError(t, err)

	// Find requests total metric family
	var requestsFamily *dto.MetricFamily
	for _, family := range gathered {
		if family.GetName() == "gokvs_http_requests_total" {
			requestsFamily = family
			break
		}
	}

	assert.NotNil(t, requestsFamily, "Should find requests total metric family")
	assert.Equal(t, 3, len(requestsFamily.GetMetric()), "Should have 3 labeled metrics")

	// Verify each metric has correct labels
	for _, metric := range requestsFamily.GetMetric() {
		labels := metric.GetLabel()
		assert.Equal(t, 2, len(labels), "Each metric should have 2 labels")

		// Verify label names
		labelNames := make(map[string]bool)
		for _, label := range labels {
			labelNames[label.GetName()] = true
		}
		assert.True(t, labelNames["status"], "Should have status label")
		assert.True(t, labelNames["method"], "Should have method label")
	}
}

// Helper functions for metric value extraction

func getMetricValue(families []*dto.MetricFamily, metricName string) float64 {
	for _, family := range families {
		if family.GetName() == metricName {
			if len(family.GetMetric()) > 0 {
				metric := family.GetMetric()[0]
				if metric.GetCounter() != nil {
					return metric.GetCounter().GetValue()
				}
				if metric.GetGauge() != nil {
					return metric.GetGauge().GetValue()
				}
			}
		}
	}
	return 0
}

func getHistogramCount(families []*dto.MetricFamily, metricName string) uint64 {
	for _, family := range families {
		if family.GetName() == metricName {
			if len(family.GetMetric()) > 0 {
				metric := family.GetMetric()[0]
				if metric.GetHistogram() != nil {
					return metric.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return 0
}

func getHistogramSum(families []*dto.MetricFamily, metricName string) float64 {
	for _, family := range families {
		if family.GetName() == metricName {
			if len(family.GetMetric()) > 0 {
				metric := family.GetMetric()[0]
				if metric.GetHistogram() != nil {
					return metric.GetHistogram().GetSampleSum()
				}
			}
		}
	}
	return 0
}
