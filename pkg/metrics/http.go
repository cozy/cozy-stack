package metrics

import "github.com/prometheus/client_golang/prometheus"

// HTTPTotalDurations is a summary metric of the durations of http requests,
// labelled by method and status code
var HTTPTotalDurations = prometheus.NewSummaryVec(
	prometheus.SummaryOpts{
		Namespace: "http",
		Subsystem: "all",
		Name:      "total_duration",

		Help: "Durations of http requests, labelled by method and status code",

		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	},
	[]string{"method", "code"},
)

func init() {
	prometheus.MustRegister(HTTPTotalDurations)
}
