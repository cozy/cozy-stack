package client

import (
	"io"
	"net/url"
	"strconv"

	"github.com/cozy/cozy-stack/client/request"
)

// ProfileHeap returns a sampling of memory allocations as pprof format.
func (ac *AdminClient) ProfileHeap() (io.ReadCloser, error) {
	res, err := ac.Req(&request.Options{
		Method: "GET",
		Path:   "/tools/pprof/heap",
	})
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}

// ProfileGoroutine returns the stack traces of all current goroutines as
// pprof format (debug=0) or text format (debug=1 or debug=2).
func (ac *AdminClient) ProfileGoroutine(debug ...int) (io.ReadCloser, error) {
	opts := &request.Options{
		Method: "GET",
		Path:   "/tools/pprof/goroutine",
	}
	if len(debug) > 0 && debug[0] > 0 {
		opts.Queries = url.Values{
			"debug": {strconv.Itoa(debug[0])},
		}
	}
	res, err := ac.Req(opts)
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}
