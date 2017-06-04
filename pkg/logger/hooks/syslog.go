// +build !windows

package hooks

import (
	"io/ioutil"
	std_syslog "log/syslog"

	logrus_syslog "github.com/Sirupsen/logrus/hooks/syslog"
)

func SetupSyslog() error {
	hook, err := logrus_syslog.NewSyslogHook("", "", std_syslog.LOG_INFO, "cozy")
	if err != nil {
		return err
	}
	logrus.AddHook(hook)
	logrus.SetOutput(ioutil.Discard)
}
