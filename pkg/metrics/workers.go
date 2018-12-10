package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// WorkerExecResultSuccess for success result label
	WorkerExecResultSuccess = "success"
	// WorkerExecResultErrored for errored result label
	WorkerExecResultErrored = "errored"
)

// WorkerExecDurations is a histogram metric of the execution duration in
// seconds of the workers labelled by worker type and result.
var WorkerExecDurations = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: "workers",
		Subsystem: "exec",
		Name:      "durations",

		Help: "Execution duration in seconds of the workers labelled by worker type and result.",

		// A 30 seconds of granularity should be hopefully be enough. With 10
		// buckets, it gives us a range from 0 to 5 minutes. We may readjust these
		// parameters when we gather more metrics.
		Buckets: prometheus.LinearBuckets(0, 30, 10),
	},
	[]string{"worker_type", "result"},
)

// WorkerExecCounter is a counter number of total executions, without counting
// retries, of the workers labelled by worker type and result.
var WorkerExecCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "workers",
		Subsystem: "exec",
		Name:      "count",

		Help: `Number of total executions, without counting retries, of the workers labelled by
worker type and result. This should be equivalent to the number of jobs consumed
from the queue.`,
	},
	[]string{"worker_type", "result"},
)

// WorkerKonnectorExecDeleteCounter is a counter number of total executions, without counting
// retries, of the konnectors jobs with the "accound_deleted: true" parameter
var WorkerKonnectorExecDeleteCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "workers",
		Subsystem: "konnectors",
		Name:      "delete_count",

		Help: `Number of konnectors executions, with the "account_deleted: true" parameter`,
	},
	[]string{"worker_type", "result"},
)

// WorkerExecTimeoutsCounter is a counter number of total timeouts,
// labelled by worker type and slug.
var WorkerExecTimeoutsCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "workers",
		Subsystem: "exec",
		Name:      "timeouts",

		Help: `Number of total timeouts, of the workers labelled by worker type and slug.`,
	},
	[]string{"worker_type", "slug"},
)

// WorkerExecRetries is a histogram metric of the number of retries of the
// workers labelled by worker type.
var WorkerExecRetries = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: "workers",
		Subsystem: "exec",
		Name:      "retries",

		Help: `Number of retries of the workers labelled by worker type.`,

		// Execution count should usually not be greater than 5.
		Buckets: prometheus.LinearBuckets(0, 1, 5),
	},
	[]string{"worker_type"},
)

// WorkersKonnectorsExecDurations is a histogram metric of the number of
// execution durations of the commands executed for konnectors and services,
// labelled by application slug
var WorkersKonnectorsExecDurations = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: "workers",
		Subsystem: "konnectors",
		Name:      "durations",

		Help: `Execution durations of the commands executed for konnectors and services,
labelled by application slug. This should be a sub-duration of the
workers_exec_durations for the "konnector" and "service" worker types, but offers
a label by slug.`,

		// Using the same buckets as WorkerExecDurations
		Buckets: prometheus.LinearBuckets(0, 30, 10),
	},
	[]string{"slug", "result"},
)

func init() {
	prometheus.MustRegister(
		WorkerExecDurations,
		WorkerExecCounter,
		WorkerExecRetries,
		WorkerExecTimeoutsCounter,
		WorkerKonnectorExecDeleteCounter,

		WorkersKonnectorsExecDurations,
	)
}
