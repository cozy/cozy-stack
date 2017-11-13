// +build !windows

package logger

import (
	"log/syslog"

	"github.com/sirupsen/logrus"
	logrus_syslog "github.com/sirupsen/logrus/hooks/syslog"
)

func syslogHook() (logrus.Hook, error) {
	return logrus_syslog.NewSyslogHook("", "", syslog.LOG_INFO, "cozy")
}
