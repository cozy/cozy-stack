package metrics

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/prometheus/client_golang/prometheus"
)

// innerDataCollector collects data from the database, like the number of cozy
// instances. These data are global and collected on demand from the database.
type innerDataCollector struct {
	instancesCountDesc *prometheus.Desc
}

func (i *innerDataCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- i.instancesCountDesc
}

func (i *innerDataCollector) Collect(ch chan<- prometheus.Metric) {
	if count, err := couchdb.CountAllDocs(couchdb.GlobalDB, consts.Instances); err == nil {
		ch <- prometheus.MustNewConstMetric(
			i.instancesCountDesc,
			prometheus.CounterValue,
			float64(count),
		)
	}
}

func init() {
	prometheus.MustRegister(&innerDataCollector{
		instancesCountDesc: prometheus.NewDesc(
			prometheus.BuildFQName("inner_data", "instances", "count"), /* fqName*/
			"Number of existing instances.",                            /* help */
			[]string{},                                                 /* variableLabels */
			prometheus.Labels{},                                        /* constLabels */
		),
	})
}
