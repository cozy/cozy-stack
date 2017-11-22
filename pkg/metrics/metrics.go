package metrics

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"

	"github.com/labstack/echo"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// innerDataCollector collects data from the database, like the number of cozy
// instances. These data are global and collected on demand from the database.
type innerDataCollector struct {
	instancesCountDesc *prometheus.Desc
}

func newInnerDataCollector() prometheus.Collector {
	return &innerDataCollector{
		instancesCountDesc: prometheus.NewDesc(
			"inner_data_instances_count",  /* fqName*/
			"Number of created instances", /* help */
			[]string{},                    /* variableLabels */
			prometheus.Labels{},           /* constLabels */
		),
	}
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

// Routes set the /metrics routes.
//
// Default prometheus handler comes with two collectors:
//  - ProcessCollector: cpu, memory and file descriptor usage as well as the
//    process start time for the given process id under the given
//    namespace...
//  - GoCollector: current go process, goroutines, GC pauses, ...
func Routes(g *echo.Group) {
	g.GET("", echo.WrapHandler(promhttp.Handler()))
}

func init() {
	prometheus.MustRegister(newInnerDataCollector())
}
