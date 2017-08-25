// +build windows

package logger

import (
	"errors"

	"github.com/sirupsen/logrus"
)

func syslogHook() (logrus.Hook, error) {
	return nil, errors.New("Syslog is not available on Windows")
}
