package cmd

import (
	"bytes"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/stack"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmd(t *testing.T) {
	if testing.Short() {
		t.Skip("a redis is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()

	_, err := couchdb.CheckStatus()
	require.NoError(t, err, "This test need couchdb to run.")

	_, err = stack.Start()
	require.NoError(t, err)

	tempdir, err := os.MkdirTemp("", "cozy-stack")
	require.NoError(t, err)

	config.GetConfig().Fs.URL = &url.URL{
		Scheme: "file",
		Host:   "localhost",
		Path:   tempdir,
	}
	server := echo.New()
	err = web.SetupRoutes(server)
	require.NoError(t, err, "Could not start server")

	ts := httptest.NewServer(server)
	u, _ := url.Parse(ts.URL)
	domain := strings.ReplaceAll(u.Host, "127.0.0.1", "localhost")

	_ = lifecycle.Destroy(domain)
	testInstance, err := lifecycle.Create(&lifecycle.Options{
		Domain: domain,
		Locale: "en",
	})
	require.NoError(t, err, "could not create test instance")

	// Cleanup
	t.Cleanup(func() {
		_ = lifecycle.Destroy("test-files")
		os.RemoveAll(tempdir)
		ts.Close()
	})

	token, err := testInstance.MakeJWT(consts.CLIAudience, "CLI", consts.Files, "", time.Now())
	require.NoError(t, err, "could not get test instance token")

	testClient := &client.Client{
		Domain:     domain,
		Authorizer: &request.BearerAuthorizer{Token: token},
	}

	buf := new(bytes.Buffer)
	err = execCommand(testClient, "mkdir /hello-test", buf)
	assert.NoError(t, err)

	buf = new(bytes.Buffer)
	err = execCommand(testClient, "ls /", buf)
	assert.NoError(t, err)
	assert.True(t, bytes.Contains(buf.Bytes(), []byte("hello-test")))
}
