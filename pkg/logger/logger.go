package logger

import "github.com/Sirupsen/logrus"

// WithNamespace returns a logger with the specified nspace field.
func WithNamespace(nspace string) *logrus.Entry {
	return logrus.WithField("nspace", nspace)
}

// WithDomain returns a logger with the specified domain field.
func WithDomain(domain string) *logrus.Entry {
	return logrus.WithField("domain", domain)
}

// IsDebug returns whether or not the debug mode is activated.
func IsDebug() bool {
	return logrus.GetLevel() == logrus.DebugLevel
}
