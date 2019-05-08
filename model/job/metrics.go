package job

import "github.com/prometheus/client_golang/prometheus"

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
	broker := globalJobSystem
	for _, workerType := range broker.WorkersTypes() {
		count, err := broker.WorkerQueueLen(workerType)
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
