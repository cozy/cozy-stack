package logger

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

var debugLogger *logrus.Logger

var domains = new(sync.Map)

// Fields type, used to pass to [Logger.WithFields].
type Fields map[string]interface{}

// Logger allows to emits logs to the divers log systems.
type Logger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})

	Debug(msg string)
	Info(msg string)
	Warn(msg string)
	Error(msg string)

	WithField(fn string, fv interface{}) Logger
	WithFields(fields Fields) Logger
	WithTime(t time.Time) Logger

	Log(level Level, msg string)
}

// Options contains the configuration values of the logger system
type Options struct {
	Hooks  []logrus.Hook
	Output io.Writer
	Level  string
	Redis  redis.UniversalClient
}

// Init initializes the logger module with the specified options.
//
// It also setup the global logger for go-redis. Thoses are at
// Info level.
func Init(opt Options) error {
	level := opt.Level
	if level == "" {
		level = "info"
	}
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}

	// Setup the global logger in case of someone call the global functions.
	setupLogger(logrus.StandardLogger(), logLevel, opt)

	// Setup the debug logger used for the the domains in debug mode.
	debugLogger = logrus.New()
	setupLogger(debugLogger, logrus.DebugLevel, opt)

	w := WithNamespace("go-redis").Writer()
	l := log.New(w, "", 0)
	redis.SetLogger(&contextPrint{l})

	err = initDebugger(opt.Redis)
	if err != nil {
		return err
	}
	return nil
}

// Entry is the struct on which we can call the Debug, Info, Warn, Error
// methods with the structured data accumulated.
type Entry struct {
	entry *logrus.Entry
}

// WithDomain returns a logger with the specified domain field.
func WithDomain(domain string) *Entry {
	e := logrus.WithField("domain", domain)
	return &Entry{e}
}

// WithNamespace returns a logger with the specified nspace field.
func WithNamespace(nspace string) *Entry {
	entry := logrus.WithField("nspace", nspace)

	return &Entry{entry}
}

// WithNamespace adds a namespace (nspace field).
func (e *Entry) WithNamespace(nspace string) *Entry {
	entry := e.entry.WithField("nspace", nspace)
	return &Entry{entry}
}

// WithDomain add a domain field.
func (e *Entry) WithDomain(domain string) *Entry {
	entry := e.entry.WithField("domain", domain)
	return &Entry{entry}
}

// WithField adds a single field to the Entry.
func (e *Entry) WithField(key string, value interface{}) Logger {
	entry := e.entry.WithField(key, value)
	return &Entry{entry}
}

// WithFields adds a map of fields to the Entry.
func (e *Entry) WithFields(fields Fields) Logger {
	entry := e.entry.WithFields(logrus.Fields(fields))
	return &Entry{entry}
}

// WithTime overrides the Entry's time
func (e *Entry) WithTime(t time.Time) Logger {
	entry := e.entry.WithTime(t)
	return &Entry{entry}
}

// Clone clones a logger entry.
func (e *Entry) AddHook(hook logrus.Hook) {
	// We need to clone the underlying logger in order to add a specific hook
	// only on this logger.
	in := e.entry.Logger
	cloned := &logrus.Logger{
		Out:       in.Out,
		Hooks:     make(logrus.LevelHooks, len(in.Hooks)),
		Formatter: in.Formatter,
		Level:     in.Level,
	}
	for k, v := range in.Hooks {
		cloned.Hooks[k] = v
	}
	cloned.AddHook(hook)
	e.entry.Logger = cloned
}

// maxLineWidth limits the number of characters of a line of log to avoid issue
// with syslog.
const maxLineWidth = 2000

func (e *Entry) Log(level Level, msg string) {
	if len(msg) > maxLineWidth {
		msg = msg[:maxLineWidth-12] + " [TRUNCATED]"
	}

	domain, haveDomain := e.entry.Data["domain"]

	if haveDomain && level == DebugLevel && debugger.ExpiresAt(domain.(string)) != nil {
		// The domain is listed in the debug domains and the ttl is valid, use the debuglogger
		// to debug
		debugLogger.WithFields(e.entry.Data).Log(logrus.DebugLevel, msg)
		return
	}

	e.entry.Log(getLogrusLevel(level), msg)
}

func (e *Entry) Debug(msg string) {
	e.Log(DebugLevel, msg)
}

func (e *Entry) Info(msg string) {
	e.Log(InfoLevel, msg)
}

func (e *Entry) Warn(msg string) {
	e.Log(WarnLevel, msg)
}

func (e *Entry) Error(msg string) {
	e.Log(ErrorLevel, msg)
}

func (e *Entry) Debugf(format string, args ...interface{}) {
	e.Debug(fmt.Sprintf(format, args...))
}

func (e *Entry) Infof(format string, args ...interface{}) {
	e.Info(fmt.Sprintf(format, args...))
}

func (e *Entry) Warnf(format string, args ...interface{}) {
	e.Warn(fmt.Sprintf(format, args...))
}

func (e *Entry) Errorf(format string, args ...interface{}) {
	e.Error(fmt.Sprintf(format, args...))
}

func (e *Entry) Writer() *io.PipeWriter {
	return e.entry.Writer()
}

// IsDebug returns whether or not the debug mode is activated.
func (e *Entry) IsDebug() bool {
	return e.entry.Logger.Level == logrus.DebugLevel
}

func setupLogger(logger *logrus.Logger, lvl logrus.Level, opt Options) {
	logger.SetLevel(lvl)

	if opt.Output != nil {
		logger.SetOutput(opt.Output)
	}

	// We need to reset the hooks to avoid the accumulation of hooks for
	// the global loggers in case of several calls to `Init`.
	//
	// This is the case for `logrus.StandardLogger()` and the tests for example.
	logger.Hooks = logrus.LevelHooks{}

	for _, hook := range opt.Hooks {
		logger.AddHook(hook)
	}

	if build.IsDevRelease() && lvl == logrus.DebugLevel {
		formatter := logger.Formatter.(*logrus.TextFormatter)
		formatter.TimestampFormat = time.RFC3339Nano
	}
}

type contextPrint struct {
	l *log.Logger
}

func (c contextPrint) Printf(ctx context.Context, format string, args ...interface{}) {
	c.l.Printf(format, args...)
}

func getLogrusLevel(lvl Level) logrus.Level {
	var logrusLevel logrus.Level
	switch lvl {
	case DebugLevel:
		logrusLevel = logrus.DebugLevel
	case InfoLevel:
		logrusLevel = logrus.InfoLevel
	case WarnLevel:
		logrusLevel = logrus.WarnLevel
	default:
		logrusLevel = logrus.ErrorLevel
	}

	return logrusLevel
}
