//go:build windows
// +build windows

package logger

import (
	"errors"

	"github.com/sirupsen/logrus"
)

func syslogHook() (logrus.Hook, error) {
	return nil, errors.New("syslog is not available on Windows")
}
