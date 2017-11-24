package globals

import (
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/scheduler"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	broker jobs.Broker
	schder scheduler.Scheduler
)

// GetBroker returns the global job broker.
func GetBroker() jobs.Broker {
	if broker == nil {
		panic("Job system not initialized")
	}
	return broker
}

// GetScheduler returns the global job scheduler.
func GetScheduler() scheduler.Scheduler {
	if schder == nil {
		panic("Job system not initialized")
	}
	return schder
}

// Set will set the globales values.
func Set(b jobs.Broker, s scheduler.Scheduler) {
	broker = b
	schder = s
}

type workersQueuesCollector struct {
	prometheus.Desc
}

func newWorkersQueuesCollector() prometheus.Collector {
	desc := prometheus.NewDesc(
		prometheus.BuildFQName("workers", "queues", "len"),
		`Len of the workers queues by worker type`,
		[]string{"worker_type"},
		prometheus.Labels{},
	)
	return &workersQueuesCollector{*desc}
}

func (i *workersQueuesCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- &i.Desc
}

func (i *workersQueuesCollector) Collect(ch chan<- prometheus.Metric) {
	broker := GetBroker()
	for _, workerType := range broker.WorkersTypes() {
		count, err := broker.QueueLen(workerType)
		if err != nil {
			continue
		}
		ch <- prometheus.MustNewConstMetric(
			&i.Desc, prometheus.GaugeValue, float64(count),
			workerType,
		)
	}
}

func init() {
	prometheus.MustRegister(newWorkersQueuesCollector())
}
