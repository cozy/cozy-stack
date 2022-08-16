package metrics

import (
	"github.com/labstack/echo/v4"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Routes set the /metrics routes.
//
// Default prometheus handler comes with two collectors:
//   - ProcessCollector: cpu, memory and file descriptor usage as well as the
//     process start time for the given process id under the given
//     namespace...
//   - GoCollector: current go process, goroutines, GC pauses, ...
func Routes(g *echo.Group) {
	g.GET("", echo.WrapHandler(promhttp.Handler()))
}
