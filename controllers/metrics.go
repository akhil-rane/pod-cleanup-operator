package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	metricClustersDeleted = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pods_deleted_total",
			Help: "Counter incremented every time we observe a deleted pod.",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(metricClustersDeleted)
}
