package cmd

import (
	"bytes"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/stack"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()

	testutils.NeedCouchdb(t)

	_, err := stack.Start()
	require.NoError(t, err)

	tempDir := t.TempDir()

	config.GetConfig().Fs.URL = &url.URL{
		Scheme: "file",
		Host:   "localhost",
		Path:   tempDir,
	}
	server := echo.New()
	require.NoError(t, web.SetupRoutes(server))

	ts := httptest.NewServer(server)
	t.Cleanup(ts.Close)

	u, _ := url.Parse(ts.URL)
	domain := strings.ReplaceAll(u.Host, "127.0.0.1", "localhost")

	_ = lifecycle.Destroy(domain)
	testInstance, err := lifecycle.Create(&lifecycle.Options{
		Domain: domain,
		Locale: "en",
	})
	if err != nil {
		require.NoError(t, err, "Could not create test instance.")
	}
	t.Cleanup(func() { _ = lifecycle.Destroy("test-files") })

	token, err := testInstance.MakeJWT(consts.CLIAudience, "CLI", consts.Files, "", time.Now())
	if err != nil {
		require.NoError(t, err, "Could not get test instance token.")
	}

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
