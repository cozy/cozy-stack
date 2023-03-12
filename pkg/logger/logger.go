package logger

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

const (
	debugRedisAddChannel = "add:log-debug"
	debugRedisRmvChannel = "rmv:log-debug"
	debugRedisPrefix     = "debug:"
)

var opts Options
var loggers = make(map[string]domainEntry)
var loggersMu sync.RWMutex

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

	Log(level Level, msg string)
}

// Options contains the configuration values of the logger system
type Options struct {
	Syslog bool
	Level  string
	Redis  redis.UniversalClient
}

type domainEntry struct {
	log       *logrus.Logger
	expiredAt *time.Time
}

func (entry *domainEntry) Expired() bool {
	if entry.expiredAt == nil {
		return false
	}
	return entry.expiredAt.Before(time.Now())
}

// Init initializes the logger module with the specified options.
func Init(opt Options) error {
	level := opt.Level
	if level == "" {
		level = "info"
	}
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	logrus.SetLevel(logLevel)
	if opt.Syslog {
		hook, err := syslogHook()
		if err != nil {
			return err
		}
		logrus.AddHook(hook)
		logrus.SetOutput(io.Discard)
	} else if build.IsDevRelease() && logLevel == logrus.DebugLevel {
		formatter := logrus.StandardLogger().Formatter.(*logrus.TextFormatter)
		formatter.TimestampFormat = time.RFC3339Nano
	}
	if cli := opt.Redis; cli != nil {
		ctx := context.Background()
		go subscribeLoggersDebug(ctx, cli)
		go loadDebug(ctx, cli)
	}
	opts = opt
	return nil
}

// AddDebugDomain adds the specified domain to the debug list.
func AddDebugDomain(domain string, ttl time.Duration) error {
	if cli := opts.Redis; cli != nil {
		ctx := context.Background()
		return publishDebug(ctx, cli, debugRedisAddChannel, domain, ttl)
	}
	addDebugDomain(domain, ttl)
	return nil
}

// RemoveDebugDomain removes the specified domain from the debug list.
func RemoveDebugDomain(domain string) error {
	if cli := opts.Redis; cli != nil {
		ctx := context.Background()
		return publishDebug(ctx, cli, debugRedisRmvChannel, domain, 0)
	}
	removeDebugDomain(domain)
	return nil
}

// Entry is the struct on which we can call the Debug, Info, Warn, Error
// methods with the structured data accumulated.
type Entry struct {
	entry *logrus.Entry
}

// WithDomain returns a logger with the specified domain field.
func WithDomain(domain string) *Entry {
	loggersMu.RLock()
	entry, ok := loggers[domain]
	loggersMu.RUnlock()
	if ok {
		if !entry.Expired() {
			e := entry.log.WithField("domain", domain)
			return &Entry{e}
		}
		removeDebugDomain(domain)
	}
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
func (e *Entry) WithTime(t time.Time) *Entry {
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

func addDebugDomain(domain string, ttl time.Duration) {
	loggersMu.Lock()
	defer loggersMu.Unlock()
	_, ok := loggers[domain]
	if ok {
		return
	}
	logger := logrus.New()
	logger.Level = logrus.DebugLevel
	if opts.Syslog {
		hook, err := syslogHook()
		if err == nil {
			logger.Hooks.Add(hook)
			logger.Out = io.Discard
		}
	}
	expiredAt := time.Now().Add(ttl)
	loggers[domain] = domainEntry{logger, &expiredAt}
}

func removeDebugDomain(domain string) {
	loggersMu.Lock()
	defer loggersMu.Unlock()
	delete(loggers, domain)
}

func subscribeLoggersDebug(ctx context.Context, cli redis.UniversalClient) {
	sub := cli.Subscribe(ctx, debugRedisAddChannel, debugRedisRmvChannel)
	for msg := range sub.Channel() {
		parts := strings.Split(msg.Payload, "/")
		domain := parts[0]
		switch msg.Channel {
		case debugRedisAddChannel:
			var ttl time.Duration
			if len(parts) >= 2 {
				ttl, _ = time.ParseDuration(parts[1])
			}
			addDebugDomain(domain, ttl)
		case debugRedisRmvChannel:
			removeDebugDomain(domain)
		}
	}
}

func loadDebug(ctx context.Context, cli redis.UniversalClient) {
	keys, err := cli.Keys(ctx, debugRedisPrefix+"*").Result()
	if err != nil {
		return
	}
	for _, key := range keys {
		ttl, err := cli.TTL(ctx, key).Result()
		if err != nil {
			continue
		}
		domain := strings.TrimPrefix(key, debugRedisPrefix)
		addDebugDomain(domain, ttl)
	}
}

func publishDebug(ctx context.Context, cli redis.UniversalClient, channel, domain string, ttl time.Duration) error {
	err := cli.Publish(ctx, channel, domain+"/"+ttl.String()).Err()
	if err != nil {
		return err
	}
	key := debugRedisPrefix + domain
	if channel == debugRedisAddChannel {
		err = cli.Set(ctx, key, 0, ttl).Err()
	} else {
		err = cli.Del(ctx, key).Err()
	}
	return err
}

// DebugExpiration returns the expiration date for the debug mode for the
// instance logger of the given domain (or nil if the debug mode is not
// activated).
func DebugExpiration(domain string) *time.Time {
	loggersMu.RLock()
	entry, ok := loggers[domain]
	loggersMu.RUnlock()
	if !ok {
		return nil
	}
	return entry.expiredAt
}

// IsDebug returns whether or not the debug mode is activated.
func (e *Entry) IsDebug() bool {
	return e.entry.Logger.Level == logrus.DebugLevel
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
