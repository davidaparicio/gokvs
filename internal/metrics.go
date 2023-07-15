package internal

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	QueriesInflight prometheus.Gauge
	EventsReplayed  prometheus.Counter
	EventsGet       prometheus.Counter
	EventsPut       prometheus.Counter
	EventsDelete    prometheus.Counter
	HttpNotAllowed  prometheus.Counter
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
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
	}
	reg.MustRegister(m.QueriesInflight)
	reg.MustRegister(m.EventsReplayed)
	reg.MustRegister(m.EventsGet)
	reg.MustRegister(m.EventsPut)
	reg.MustRegister(m.EventsDelete)
	reg.MustRegister(m.HttpNotAllowed)
	return m
}
