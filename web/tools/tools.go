package tools

import (
	"runtime"
	"runtime/pprof"

	"github.com/labstack/echo/v4"
)

func HeapProfiling(c echo.Context) error {
	res := c.Response()
	res.Header().Set(echo.HeaderContentType, echo.MIMEOctetStream)
	res.Header().Set(echo.HeaderContentDisposition, `attachment; filename="heap.pprof"`)
	runtime.GC() // get up-to-date statistics
	return pprof.WriteHeapProfile(res)
}

// Routes sets the routing for the tools (like profiling).
func Routes(router *echo.Group) {
	router.GET("/pprof/heap", HeapProfiling)
}
