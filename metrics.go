package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	alertsReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmanager_gchat_alerts_received_total",
			Help: "The total number of alerts received",
		},
		[]string{"status"},
	)

	alertsSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmanager_gchat_alerts_sent_total",
			Help: "The total number of alerts sent to Google Chat",
		},
		[]string{"status"},
	)

	alertProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "alertmanager_gchat_processing_duration_seconds",
			Help:    "Time spent processing alerts",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"status"},
	)

	providerRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "alertmanager_gchat_provider_request_duration_seconds",
			Help:    "Time spent making requests to provider",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"provider", "status"},
	)

	providerErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmanager_gchat_provider_errors_total",
			Help: "The total number of provider errors",
		},
		[]string{"provider"},
	)
)
