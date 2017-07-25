package config

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestUseViper(t *testing.T) {
	cfg := viper.New()
	cfg.Set("couchdb.url", "http://db:1234")
	UseViper(cfg)
	assert.Equal(t, "http://db:1234/", CouchURL().String())
}

func TestSetup(t *testing.T) {
	tmpdir := os.TempDir()
	tmpfile, err := os.OpenFile(filepath.Join(tmpdir, "cozy.yaml"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if !assert.NoError(t, err) {
		return
	}
	defer tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	os.Setenv("OS_USERNAME", "os_username_val")
	os.Setenv("OS_PASSWORD", "os_password_val")
	os.Setenv("OS_PROJECT_NAME", "os_project_name_val")
	os.Setenv("OS_USER_DOMAIN_NAME", "os_user_domain_name_val")
	os.Setenv("MAIL_USERNAME", "mail_username_val")
	os.Setenv("MAIL_PASSWORD", "mail_password_val")

	_, err = tmpfile.Write([]byte(`
# cozy-stack configuration file

# server host - flags: --host
host: myhost
# server port - flags: --port -p
port: 1235


fs:
  # file system url - flags: --fs-url
  # default url is the directory relative to the binary: ./storage

  url: swift://openstack/?UserName={{ .Env.OS_USERNAME }}&Password={{ .Env.OS_PASSWORD }}&ProjectName={{ .Env.OS_PROJECT_NAME }}&UserDomainName={{ .Env.OS_USER_DOMAIN_NAME }}

couchdb:
    # couchdb url - flags: --couchdb-url
    url: http://192.168.99.100:5984/

mail:
  host: ssl0.ovh.net
  port: 465
  username: {{ .Env.MAIL_USERNAME }}
  password: {{ .Env.MAIL_PASSWORD }}


log:
    # logger level (debug, info, warning, panic, fatal) - flags: --log-level
    level: warning

registries:
  foo:
    - http://abc
    - http://def
  bar:
    - http://def
    - http://abc
  default:
    - https://default
`))
	if !assert.NoError(t, err) {
		return
	}
	err = Setup(tmpfile.Name())
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, "myhost", GetConfig().Host)
	assert.Equal(t, 1235, GetConfig().Port)
	assert.Equal(t, "swift://openstack/?UserName=os_username_val&Password=os_password_val&ProjectName=os_project_name_val&UserDomainName=os_user_domain_name_val", GetConfig().Fs.URL.String())
	assert.Equal(t, "http://192.168.99.100:5984/", GetConfig().CouchDB.URL.String())
	assert.Equal(t, "ssl0.ovh.net", GetConfig().Mail.Host)
	assert.Equal(t, 465, GetConfig().Mail.Port)
	assert.Equal(t, "mail_username_val", GetConfig().Mail.Username)
	assert.Equal(t, "mail_password_val", GetConfig().Mail.Password)
	assert.Equal(t, logrus.GetLevel(), logrus.WarnLevel)

	assert.EqualValues(t, []string{"http://abc", "http://def", "https://default"}, regsToStrings(GetConfig().Registries["foo"]))
	assert.EqualValues(t, []string{"http://def", "http://abc", "https://default"}, regsToStrings(GetConfig().Registries["bar"]))
	assert.EqualValues(t, []string{"https://default"}, regsToStrings(GetConfig().Registries["default"]))
}

func regsToStrings(regs []*url.URL) []string {
	ss := make([]string, len(regs))
	for i, r := range regs {
		ss[i] = r.String()
	}
	return ss
}
