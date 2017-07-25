package utils

import "io"

type readCloser struct {
	io.Reader
	c func() error
}

// ReadCloser returns an io.ReadCloser from a io.Reader and a close method.
func ReadCloser(r io.Reader, c func() error) io.ReadCloser {
	return &readCloser{r, c}
}

func (r *readCloser) Read(p []byte) (int, error) {
	return r.Reader.Read(p)
}

func (r *readCloser) Close() error {
	return r.c()
}
