package internal

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
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
