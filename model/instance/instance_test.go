package instance_test

import (
	"os"
	"testing"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/stretchr/testify/assert"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

func TestSubdomain(t *testing.T) {
	inst := &instance.Instance{
		Domain: "foo.example.com",
	}
	cfg := config.GetConfig()
	was := cfg.Subdomains
	defer func() { cfg.Subdomains = was }()

	cfg.Subdomains = config.NestedSubdomains
	u := inst.SubDomain("calendar")
	assert.Equal(t, "https://calendar.foo.example.com/", u.String())

	cfg.Subdomains = config.FlatSubdomains
	u = inst.SubDomain("calendar")
	assert.Equal(t, "https://foo-calendar.example.com/", u.String())
}

func TestBuildAppToken(t *testing.T) {
	manifest := &app.WebappManifest{
		DocID:   consts.Apps + "/my-app",
		DocSlug: "my-app",
	}
	inst := &instance.Instance{
		Domain:        "test-ctx-token.example.com",
		SessionSecret: crypto.GenerateRandomBytes(64),
	}

	tokenString := inst.BuildAppToken(manifest.Slug(), "sessionid")
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		_, ok := token.Method.(*jwt.SigningMethodHMAC)
		assert.True(t, ok, "The signing method should be HMAC")
		return inst.SessionSecret, nil
	})
	assert.NoError(t, err)
	assert.True(t, token.Valid)

	claims, ok := token.Claims.(jwt.MapClaims)
	assert.True(t, ok, "Claims can be parsed as standard claims")
	assert.Equal(t, "app", claims["aud"])
	assert.Equal(t, "test-ctx-token.example.com", claims["iss"])
	assert.Equal(t, "my-app", claims["sub"])
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	res := m.Run()
	os.Exit(res)
}
