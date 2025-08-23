package internal

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	QueriesInflight          prometheus.Gauge
	EventsReplayed           prometheus.Counter
	EventsGet                prometheus.Counter
	EventsPut                prometheus.Counter
	EventsDelete             prometheus.Counter
	HttpNotAllowed           prometheus.Counter
	RequestsTotal            *prometheus.CounterVec
	RequestDurationHistogram *prometheus.HistogramVec
	Info                     *prometheus.GaugeVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		Info: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Subsystem: "gokvs",
			Name:      "info",
			Help:      "Information about the GoKVs environment",
		}, []string{"version"}),
		QueriesInflight: prometheus.NewGauge(prometheus.GaugeOpts{
			Subsystem: "gokvs",
			Name:      "queries_inflight",
			Help:      "total queries inflight",
		}),
		EventsReplayed: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: "gokvs",
			Name:      "events_replayed",
			Help:      "total events replayed before starting",
		}),
		EventsGet: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: "gokvs",
			Name:      "events_get",
			Help:      "total events GET",
		}),
		EventsPut: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: "gokvs",
			Name:      "events_put",
			Help:      "total events PUT",
		}),
		EventsDelete: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: "gokvs",
			Name:      "events_delete",
			Help:      "total events DELETE",
		}),
		HttpNotAllowed: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: "http",
			Name:      "405",
			Help:      "total Not Allowed HTTP Error",
		}),
		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "total HTTP requests processed",
		}, []string{"code", "method"}),
		RequestDurationHistogram: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "Seconds spent serving HTTP requests.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"code", "method"}), //[]string{"path"})
	}
	reg.MustRegister(m.Info)
	reg.MustRegister(m.QueriesInflight)
	reg.MustRegister(m.EventsReplayed)
	reg.MustRegister(m.EventsGet)
	reg.MustRegister(m.EventsPut)
	reg.MustRegister(m.EventsDelete)
	reg.MustRegister(m.HttpNotAllowed)
	reg.MustRegister(m.RequestsTotal)
	reg.MustRegister(m.RequestDurationHistogram)
	return m
}
