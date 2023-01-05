package files

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestPermissions(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	t.Run("CreateDirNoToken", func(t *testing.T) {
		noToken := ""
		res, err := request("POST", "/files/?Name=icantcreateyou&Type=directory", noToken, strings.NewReader(""))
		assert.NoError(t, err)
		assert.Equal(t, 401, res.StatusCode)
	})

	t.Run("CreateDirBadType", func(t *testing.T) {
		badtok, _ := testInstance.MakeJWT(consts.AccessTokenAudience, clientID, "io.cozy.events", "", time.Now())
		res, err := request("POST", "/files/?Name=icantcreateyou&Type=directory", badtok, strings.NewReader(""))
		assert.NoError(t, err)
		assert.Equal(t, 403, res.StatusCode)
	})

	t.Run("CreateDirWildCard", func(t *testing.T) {
		wildTok, _ := testInstance.MakeJWT(consts.AccessTokenAudience, clientID, "io.cozy.files.*", "", time.Now())
		res, err := request("POST", "/files/?Name=icantcreateyou&Type=directory", wildTok, strings.NewReader(""))
		assert.NoError(t, err)
		assert.Equal(t, 201, res.StatusCode)
	})

	t.Run("CreateDirLimitedScope", func(t *testing.T) {
		res, data := createDir(t, "/files/?Name=permissionholder&Type=directory")
		assert.Equal(t, 201, res.StatusCode)
		id := data["data"].(map[string]interface{})["id"].(string)
		badtok, _ := testInstance.MakeJWT(consts.AccessTokenAudience, clientID, "io.cozy.files:ALL:"+id, "", time.Now())

		// not in authorized dir
		res, err := request("POST", "/files/?Name=icantcreateyou&Type=directory", badtok, strings.NewReader(""))
		assert.NoError(t, err)
		assert.Equal(t, 403, res.StatusCode)

		// in authorized dir
		res2, err := request("POST", "/files/"+id+"?Name=icancreateyou&Type=directory", token, strings.NewReader(""))
		assert.NoError(t, err)
		assert.Equal(t, 201, res2.StatusCode)
	})

	t.Run("CreateDirBadVerb", func(t *testing.T) {
		badtok, _ := testInstance.MakeJWT(consts.AccessTokenAudience, clientID, "io.cozy.files:GET", "", time.Now())
		res, err := request("POST", "/files/?Name=icantcreateyou&Type=directory", badtok, strings.NewReader(""))
		assert.NoError(t, err)
		assert.Equal(t, 403, res.StatusCode)
	})
}

func request(m, path, token string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(m, ts.URL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "text/plain")
	if token != "" {
		req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	}
	return http.DefaultClient.Do(req)
}
