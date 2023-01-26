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
