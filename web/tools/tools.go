package tools

import (
	"runtime"
	"runtime/pprof"
	"strconv"

	"github.com/labstack/echo/v4"
)

func HeapProfiling(c echo.Context) error {
	res := c.Response()
	res.Header().Set(echo.HeaderContentType, echo.MIMEOctetStream)
	res.Header().Set(echo.HeaderContentDisposition, `attachment; filename="heap.pprof"`)
	runtime.GC() // get up-to-date statistics
	return pprof.WriteHeapProfile(res)
}

func GoroutineProfiling(c echo.Context) error {
	debug, _ := strconv.Atoi(c.QueryParam("debug"))
	res := c.Response()
	if debug > 0 {
		res.Header().Set(echo.HeaderContentType, echo.MIMETextPlainCharsetUTF8)
	} else {
		res.Header().Set(echo.HeaderContentType, echo.MIMEOctetStream)
		res.Header().Set(echo.HeaderContentDisposition, `attachment; filename="goroutine.pprof"`)
	}
	return pprof.Lookup("goroutine").WriteTo(res, debug)
}

// Routes sets the routing for the tools (like profiling).
func Routes(router *echo.Group) {
	router.GET("/pprof/heap", HeapProfiling)
	router.GET("/pprof/goroutine", GoroutineProfiling)
}
