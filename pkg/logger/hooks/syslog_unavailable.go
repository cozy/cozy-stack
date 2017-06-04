// +build windows

package hooks

import (
	"errors"
)

func SetupSyslog() error {
  return errors.New("Syslog is not available on Windows!")
}
