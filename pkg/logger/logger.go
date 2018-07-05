package logger

import (
	"io/ioutil"
	"sync"

	"github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
)

const (
	debugRedisAddChannel = "add:log-debug"
	debugRedisRmvChannel = "rmv:log-debug"
)

var opts Options

var loggers = make(map[string]*logrus.Logger)
var loggersMu sync.RWMutex

// Options contains the configuration values of the logger system
type Options struct {
	Syslog bool
	Level  string
	Redis  redis.UniversalClient
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
		logrus.SetOutput(ioutil.Discard)
	}
	if cli := opt.Redis; cli != nil {
		go subscribeLoggersDebug(cli)
	}
	opts = opt
	return nil
}

// Clone clones a logrus.Logger struct.
func Clone(in *logrus.Logger) *logrus.Logger {
	out := &logrus.Logger{
		Out:       in.Out,
		Hooks:     make(logrus.LevelHooks),
		Formatter: in.Formatter,
		Level:     in.Level,
	}
	for k, v := range in.Hooks {
		out.Hooks[k] = v
	}
	return out
}

// AddDebugDomain adds the specified domain to the debug list.
func AddDebugDomain(domain string) error {
	if cli := opts.Redis; cli != nil {
		return publishLoggersDebug(cli, debugRedisAddChannel, domain)
	}
	addDebugDomain(domain)
	return nil
}

// RemoveDebugDomain removes the specified domain from the debug list.
func RemoveDebugDomain(domain string) error {
	if cli := opts.Redis; cli != nil {
		return publishLoggersDebug(cli, debugRedisRmvChannel, domain)
	}
	removeDebugDomain(domain)
	return nil
}

// WithNamespace returns a logger with the specified nspace field.
func WithNamespace(nspace string) *logrus.Entry {
	return logrus.WithField("nspace", nspace)
}

// WithDomain returns a logger with the specified domain field.
func WithDomain(domain string) *logrus.Entry {
	loggersMu.RLock()
	defer loggersMu.RUnlock()
	if logger, ok := loggers[domain]; ok {
		return logger.WithField("domain", domain)
	}
	return logrus.WithField("domain", domain)
}

func addDebugDomain(domain string) {
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
			logger.Out = ioutil.Discard
		}
	}
	loggers[domain] = logger
}

func removeDebugDomain(domain string) {
	loggersMu.Lock()
	defer loggersMu.Unlock()
	delete(loggers, domain)
}

func subscribeLoggersDebug(cli redis.UniversalClient) {
	sub := cli.Subscribe(debugRedisAddChannel, debugRedisRmvChannel)
	for msg := range sub.Channel() {
		domain := msg.Payload
		switch msg.Channel {
		case debugRedisAddChannel:
			addDebugDomain(domain)
		case debugRedisRmvChannel:
			removeDebugDomain(domain)
		}
	}
}

func publishLoggersDebug(cli redis.UniversalClient, channel, domain string) error {
	cmd := cli.Publish(channel, domain)
	return cmd.Err()
}

// IsDebug returns whether or not the debug mode is activated.
func IsDebug(logger *logrus.Entry) bool {
	return logger.Logger.Level == logrus.DebugLevel
}
