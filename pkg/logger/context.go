package logger

import "context"

// loggerContextKey is the private key type used to stash a Logger on a
// context.Context. Keeping it unexported guarantees no other package can
// collide with the same key.
type loggerContextKey struct{}

// WithContext returns a copy of ctx with log attached. Helper functions
// deep in a call stack can then retrieve the caller's request-scoped
// logger via FromContext instead of taking an explicit parameter.
func WithContext(ctx context.Context, log Logger) context.Context {
	if log == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerContextKey{}, log)
}

// FromContext returns the Logger previously attached with WithContext.
// If ctx carries no logger, it falls back to a fresh "default" namespace
// logger so callers can log unconditionally without nil-checking.
func FromContext(ctx context.Context) Logger {
	if ctx != nil {
		if log, ok := ctx.Value(loggerContextKey{}).(Logger); ok && log != nil {
			return log
		}
	}
	return WithNamespace("default")
}
