package utils

import "io"

type readCloser struct {
	io.Reader
	c func() error
}

type writeCloser struct {
	io.Writer
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
	if r.c != nil {
		return r.c()
	}
	return nil
}

// WriteCloser returns an io.WriteCloser from a io.Writer and a close method.
func WriteCloser(w io.Writer, c func() error) io.WriteCloser {
	return &writeCloser{w, c}
}

func (w *writeCloser) Read(p []byte) (int, error) {
	return w.Writer.Write(p)
}

func (w *writeCloser) Close() error {
	if w.c != nil {
		return w.c()
	}
	return nil
}
