package config

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/gomail"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigUnmarshal(t *testing.T) {
	require.NoError(t, Setup("./testdata/full_config.yaml"))

	cfg := GetConfig()

	assert.Equal(t, cfg.Host, "0.0.0.0")
	assert.Equal(t, cfg.Port, 8080)
	assert.Equal(t, cfg.AdminHost, "127.0.0.1")
	assert.Equal(t, cfg.AdminPort, 0)
	assert.Equal(t, cfg.AlertAddr, "foo2@bar.baz")
	assert.Equal(t, cfg.NoReplyAddr, "foo@bar.baz")
	assert.Equal(t, cfg.NoReplyName, "My Cozy")
	assert.Equal(t, cfg.ReplyTo, "support@cozycloud.cc")
	assert.Equal(t, cfg.GeoDB, "/geo/db/path")
	assert.Equal(t, cfg.PasswordResetInterval, time.Hour)

	// Assets
	assert.Equal(t, true, cfg.AssetsPollingDisabled)
	assert.Equal(t, time.Hour, cfg.AssetsPollingInterval)
	assert.Equal(t, "some/assets/path", cfg.Assets)
	assert.Equal(t, map[string]string{"bank_classifier": "https://some-remote-assets-url"}, cfg.RemoteAssets)

	// FS
	fsURL, err := url.Parse("https://some-url")
	require.NoError(t, err)
	assert.Equal(t, fsURL, cfg.Fs.URL)
	assert.Equal(t, 2, cfg.Fs.DefaultLayout)
	assert.Equal(t, true, cfg.Fs.CanQueryInfo)
	assert.Equal(t, FsVersioning{
		MaxNumberToKeep:            4,
		MinDelayBetweenTwoVersions: time.Minute,
	}, cfg.Fs.Versioning)

	// Jobs
	one := 1
	oneHour := time.Hour
	assert.Equal(t, "some-cmd", cfg.Jobs.ImageMagickConvertCmd)
	assert.Equal(t, "1H", cfg.Jobs.DefaultDurationToKeep)
	assert.Equal(t, true, cfg.Jobs.AllowList)
	assert.EqualValues(t, []Worker{
		{
			WorkerType:   "updates",
			Concurrency:  &one,
			MaxExecCount: &one,
			Timeout:      &oneHour,
		},
	}, cfg.Jobs.Workers)

	assert.Equal(t, "some-cmd", cfg.Konnectors.Cmd)
	assert.Equal(t, "http://some-url", cfg.Move.URL)

	// Notifications
	assert.EqualValues(t, Notifications{
		Development:            true,
		FCMServer:              "http://some-server",
		AndroidAPIKey:          "some-api-key",
		IOSCertificateKeyPath:  "cert-key-path",
		IOSCertificatePassword: "cert-password",
		IOSKeyID:               "key-id",
		IOSTeamID:              "team-id",
		HuaweiGetTokenURL:      "huawei-token",
		HuaweiSendMessagesURL:  "huawei-message",
		Contexts: map[string]SMS{
			"my-context": {
				Provider: "notif-provider",
				URL:      "https://some-notif-url",
				Token:    "some-token",
			},
		},
	}, cfg.Notifications)

	// Email
	assert.EqualValues(t, &gomail.DialerOptions{
		Host:                      "localhost",
		Port:                      25,
		Username:                  "some-username",
		Password:                  "some-password",
		DisableTLS:                true,
		SkipCertificateValidation: true,
	}, cfg.Mail)
	assert.EqualValues(t, map[string]interface{}{
		"my-context": map[string]interface{}{"host": "-"},
	}, cfg.MailPerContext)

	// Contexts
	assert.EqualValues(t, map[string]interface{}{
		"my-context": map[string]interface{}{
			"manager_url":              "https://manager-url",
			"onboarded_redirection":    "home/intro",
			"default_redirection":      "home/",
			"help_link":                "https://cozy.io/fr/support",
			"enable_premium_links":     false,
			"claudy_actions":           []interface{}{"desktop", "support"},
			"additional_platform_apps": []interface{}{"some-app"},
			"features": []interface{}{
				map[string]interface{}{"hide_konnector_errors": true},
				map[string]interface{}{"home.konnectors.hide-errors": true},
				map[string]interface{}{"home_hidden_apps": []interface{}{"foobar"}},
			},
			"home_logos": map[string]interface{}{
				"/logos/1.png": "Title 1",
				"/logos/2.png": "Title 2",
			},
		},
	}, cfg.Contexts)

	// Authentication
	assert.EqualValues(t, map[string]interface{}{
		"example_oidc": map[string]interface{}{
			"disable_password_authentication": true,
			"oidc": map[string]interface{}{
				"client_id":               "some-id",
				"client_secret":           "some-secret",
				"scope":                   "openid",
				"redirect_uri":            "https://some-redirect-uri",
				"authorize_url":           "https://some-authorize-url",
				"token_url":               "https://some-token-url",
				"userinfo_url":            "https://some-user-info-url",
				"logout_url":              "https://some-logout-url",
				"userinfo_instance_field": "instance-field",
			},
		},
	}, cfg.Authentication)

	// Office
	assert.EqualValues(t, map[string]Office{
		"foo": {
			OnlyOfficeURL: "https://onlyoffice-url",
			InboxSecret:   "inbox_secret",
			OutboxSecret:  "outbox_secret",
		},
	}, cfg.Office)

	// Registries
	u1, _ := url.Parse("https://registry-url-1")
	u2, _ := url.Parse("https://registry-url-2")
	assert.EqualValues(t, map[string][]*url.URL{
		"default": {},
		"example": {u1, u2},
	}, cfg.Registries)

	// Clouderies
	assert.EqualValues(t, map[string]ClouderyConfig{
		"default": {
			API: ClouderyAPI{
				URL:   "https://some-url",
				Token: "some-token",
			},
		},
	}, cfg.Clouderies)

	// CSPs
	assert.Equal(t, true, cfg.CSPDisabled)
	assert.EqualValues(t, map[string]string{
		"connect": "https://url-1 https://url-2",
		"font":    "https://fonts.gstatic.com/",
		"style":   "https://fonts.googleapis.com/",
	}, cfg.CSPAllowList)
	assert.EqualValues(t, map[string]map[string]string{
		"my-context": {
			"img":     "https://img-url",
			"script":  "https://script-url",
			"frame":   "https://frame-url",
			"connect": "https://connect-url",
		},
	}, cfg.CSPPerContext)
}

func TestUseViper(t *testing.T) {
	cfg := viper.New()
	cfg.Set("couchdb.url", "http://db:1234")
	assert.NoError(t, UseViper(cfg))
	assert.Equal(t, "http://db:1234/", CouchCluster(prefixer.GlobalCouchCluster).URL.String())
}

func TestSetup(t *testing.T) {
	tmpdir := t.TempDir()
	tmpfile, err := os.OpenFile(filepath.Join(tmpdir, "cozy.yaml"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	require.NoError(t, err)

	t.Setenv("OS_USERNAME", "os_username_val")
	t.Setenv("OS_PASSWORD", "os_password_val")
	t.Setenv("OS_PROJECT_NAME", "os_project_name_val")
	t.Setenv("OS_USER_DOMAIN_NAME", "os_user_domain_name_val")
	t.Setenv("MAIL_USERNAME", "mail_username_val")
	t.Setenv("MAIL_PASSWORD", "mail_password_val")

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
	require.NoError(t, err)

	err = Setup(tmpfile.Name())
	require.NoError(t, err)

	assert.Equal(t, "myhost", GetConfig().Host)
	assert.Equal(t, 1235, GetConfig().Port)
	assert.Equal(t, "swift://openstack/?UserName=os_username_val&Password=os_password_val&ProjectName=os_project_name_val&UserDomainName=os_user_domain_name_val", GetConfig().Fs.URL.String())
	assert.Equal(t, "ssl0.ovh.net", GetConfig().Mail.Host)
	assert.Equal(t, 465, GetConfig().Mail.Port)
	assert.Equal(t, "mail_username_val", GetConfig().Mail.Username)
	assert.Equal(t, "mail_password_val", GetConfig().Mail.Password)
	assert.Equal(t, logrus.GetLevel(), logrus.WarnLevel)

	assert.EqualValues(t, []string{"http://abc", "http://def", "https://default"}, regsToStrings(GetConfig().Registries["foo"]))
	assert.EqualValues(t, []string{"http://def", "http://abc", "https://default"}, regsToStrings(GetConfig().Registries["bar"]))
	assert.EqualValues(t, []string{"https://default"}, regsToStrings(GetConfig().Registries[DefaultInstanceContext]))
}

func regsToStrings(regs []*url.URL) []string {
	ss := make([]string, len(regs))
	for i, r := range regs {
		ss[i] = r.String()
	}
	return ss
}
