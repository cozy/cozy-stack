package files

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

func TestCreateDirNoToken(t *testing.T) {
	noToken := ""
	res, err := request("POST", "/files/?Name=icantcreateyou&Type=directory", noToken, strings.NewReader(""))
	assert.NoError(t, err)
	assert.Equal(t, 401, res.StatusCode)
}

func TestCreateDirBadType(t *testing.T) {
	token := makeBadToken("io.cozy.events")
	res, err := request("POST", "/files/?Name=icantcreateyou&Type=directory", token, strings.NewReader(""))
	assert.NoError(t, err)
	assert.Equal(t, 403, res.StatusCode)
}

func TestCreateDirLimitedScope(t *testing.T) {
	res, data := createDir(t, "/files/?Name=permissionholder&Type=directory")
	assert.Equal(t, 201, res.StatusCode)
	id := data["data"].(map[string]interface{})["id"].(string)
	token := makeBadToken("io.cozy.files:ALL:" + id)

	// not in authorized dir
	res, err := request("POST", "/files/?Name=icantcreateyou&Type=directory", token, strings.NewReader(""))
	assert.NoError(t, err)
	assert.Equal(t, 403, res.StatusCode)

	// in authorized dir
	res2, err := request("POST", "/files/"+id+"?Name=icancreateyou&Type=directory", token, strings.NewReader(""))
	assert.NoError(t, err)
	assert.Equal(t, 201, res2.StatusCode)
}

func TestCreateDirBadVerb(t *testing.T) {
	token := makeBadToken("io.cozy.files:GET")
	res, err := request("POST", "/files/?Name=icantcreateyou&Type=directory", token, strings.NewReader(""))
	assert.NoError(t, err)
	assert.Equal(t, 403, res.StatusCode)
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

func makeBadToken(scope string) string {
	t, _ := crypto.NewJWT(testInstance.OAuthSecret, permissions.Claims{
		StandardClaims: jwt.StandardClaims{
			Audience: permissions.AccessTokenAudience,
			Issuer:   testInstance.Domain,
			IssuedAt: crypto.Timestamp(),
			Subject:  clientID,
		},
		Scope: scope,
	})
	return t
}
