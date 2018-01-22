package utils

import "io"

type limitedWriter struct {
	w io.Writer
	n int64

	discard bool
}

func (l *limitedWriter) Write(p []byte) (n int, err error) {
	if l.n <= 0 {
		if l.discard {
			return len(p), nil
		}
		return 0, io.ErrShortWrite
	}
	var discarded bool
	if int64(len(p)) > l.n {
		p = p[0:l.n]
		if l.discard {
			discarded = true
		} else {
			err = io.ErrShortWrite
		}
	}
	n, errw := l.w.Write(p)
	if errw != nil {
		err = errw
	}
	l.n -= int64(n)
	if discarded {
		n = len(p)
	}
	return n, err
}

// LimitWriter works like io.LimitReader. It writes at most n bytes to the
// underlying Writer. It returns io.ErrShortWrite if more than n bytes are
// attempted to be written.
func LimitWriter(w io.Writer, n int64) io.Writer {
	return &limitedWriter{w, n, false}
}

// LimitWriterDiscard works like io.LimitReader. It writes at most n bytes to
// the underlying Writer. It does not return any error if more than n bytes are
// attempted to be written, the data is discarded.
func LimitWriterDiscard(w io.Writer, n int64) io.Writer {
	return &limitedWriter{w, n, true}
}
