package client

import (
	"io"

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
// pprof format.
func (ac *AdminClient) ProfileGoroutine() (io.ReadCloser, error) {
	res, err := ac.Req(&request.Options{
		Method: "GET",
		Path:   "/tools/pprof/goroutine",
	})
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}
