//go:build windows
// +build windows

package logger

import (
	"errors"

	"github.com/sirupsen/logrus"
)

// SyslogHook return a [logrus.Hook] sending all the logs to
// a local syslog server via a socket.
func SyslogHook() (logrus.Hook, error) {
	return nil, errors.New("Syslog is not available on Windows")
}
