//go:build !windows
// +build !windows

package logger

import (
	"log/syslog"

	"github.com/sirupsen/logrus"
	logrus_syslog "github.com/sirupsen/logrus/hooks/syslog"
)

// SyslogHook return a [logrus.Hook] sending all the logs to
// a local syslog server via a socket.
func SyslogHook() (logrus.Hook, error) {
	return logrus_syslog.NewSyslogHook("", "", syslog.LOG_INFO, "cozy")
}
