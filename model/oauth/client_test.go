package oauth_test

import (
	"os"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/cozy/cozy-stack/tests/testutils"
	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testInstance *instance.Instance

var c = &oauth.Client{
	CouchID: "my-client-id",
}

func TestCreateJWT(t *testing.T) {
	tokenString, err := c.CreateJWT(testInstance, "test", "foo:read")
	assert.NoError(t, err)

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		_, ok := token.Method.(*jwt.SigningMethodHMAC)
		assert.True(t, ok, "The signing method should be HMAC")
		return testInstance.OAuthSecret, nil
	})
	assert.NoError(t, err)
	assert.True(t, token.Valid)

	claims, ok := token.Claims.(jwt.MapClaims)
	assert.True(t, ok, "Claims can be parsed as standard claims")
	assert.Equal(t, "test", claims["aud"])
	assert.Equal(t, testInstance.Domain, claims["iss"])
	assert.Equal(t, "my-client-id", claims["sub"])
	assert.Equal(t, "foo:read", claims["scope"])
}

func TestParseJWT(t *testing.T) {
	tokenString, err := c.CreateJWT(testInstance, "refresh", "foo:read")
	assert.NoError(t, err)

	claims, ok := c.ValidToken(testInstance, consts.RefreshTokenAudience, tokenString)
	assert.True(t, ok, "The token must be valid")
	assert.Equal(t, "refresh", claims.Audience)
	assert.Equal(t, testInstance.Domain, claims.Issuer)
	assert.Equal(t, "my-client-id", claims.Subject)
	assert.Equal(t, "foo:read", claims.Scope)
}

func TestParseJWTInvalidAudience(t *testing.T) {
	tokenString, err := c.CreateJWT(testInstance, "access", "foo:read")
	assert.NoError(t, err)
	_, ok := c.ValidToken(testInstance, consts.RefreshTokenAudience, tokenString)
	assert.False(t, ok, "The token should be invalid")
}

func TestCreateClient(t *testing.T) {
	client := &oauth.Client{
		ClientName:   "foo",
		RedirectURIs: []string{"https://foobar"},
		SoftwareID:   "bar",

		NotificationPlatform:    "android",
		NotificationDeviceToken: "foobar",
	}
	assert.Nil(t, client.Create(testInstance))

	client2 := &oauth.Client{
		ClientName:   "foo",
		RedirectURIs: []string{"https://foobar"},
		SoftwareID:   "bar",

		NotificationPlatform:    "ios",
		NotificationDeviceToken: "foobar",
	}
	assert.Nil(t, client2.Create(testInstance))

	client3 := &oauth.Client{
		ClientName:   "foo",
		RedirectURIs: []string{"https://foobar"},
		SoftwareID:   "bar",
	}
	assert.Nil(t, client3.Create(testInstance))

	client4 := &oauth.Client{
		ClientName:   "foo-2",
		RedirectURIs: []string{"https://foobar"},
		SoftwareID:   "bar",
	}
	assert.Nil(t, client4.Create(testInstance))

	assert.Equal(t, "foo", client.ClientName)
	assert.Equal(t, "foo-2", client2.ClientName)
	assert.Equal(t, "foo-3", client3.ClientName)
	assert.Equal(t, "foo-2-2", client4.ClientName)
}

func TestCreateClientWithNotifications(t *testing.T) {
	goodClient := &oauth.Client{
		ClientName:   "client-5",
		RedirectURIs: []string{"https://foobar"},
		SoftwareID:   "bar",
	}
	if !assert.Nil(t, goodClient.Create(testInstance)) {
		return
	}

	{
		var err error
		goodClient, err = oauth.FindClient(testInstance, goodClient.ClientID)
		if !assert.NoError(t, err) {
			return
		}
	}

	{
		client := goodClient.Clone().(*oauth.Client)
		client.NotificationPlatform = "android"
		assert.Nil(t, client.Update(testInstance, goodClient))
	}

	{
		client := goodClient.Clone().(*oauth.Client)
		client.NotificationPlatform = "unknown"
		assert.NotNil(t, client.Update(testInstance, goodClient))
	}
}

func TestParseJWTInvalidIssuer(t *testing.T) {
	other := &instance.Instance{
		OAuthSecret: testInstance.OAuthSecret,
		Domain:      "other.example.com",
	}
	tokenString, err := c.CreateJWT(other, "refresh", "foo:read")
	assert.NoError(t, err)
	_, ok := c.ValidToken(testInstance, consts.RefreshTokenAudience, tokenString)
	assert.False(t, ok, "The token should be invalid")
}

func TestParseJWTInvalidSubject(t *testing.T) {
	other := &oauth.Client{
		CouchID: "my-other-client",
	}
	tokenString, err := other.CreateJWT(testInstance, "refresh", "foo:read")
	assert.NoError(t, err)
	_, ok := c.ValidToken(testInstance, consts.RefreshTokenAudience, tokenString)
	assert.False(t, ok, "The token should be invalid")
}

func TestParseGoodSoftwareID(t *testing.T) {
	goodClient := &oauth.Client{
		ClientName:   "client-5",
		RedirectURIs: []string{"https://foobar"},
		SoftwareID:   "registry://drive",
	}
	err := goodClient.CheckSoftwareID(testInstance)
	assert.Nil(t, err)
}

func TestParseHttpSoftwareID(t *testing.T) {
	goodClient := &oauth.Client{
		ClientName:   "client-5",
		RedirectURIs: []string{"https://foobar"},
		SoftwareID:   "https://github.com/cozy-labs/cozy-desktop",
	}
	err := goodClient.CheckSoftwareID(testInstance)
	assert.Nil(t, err)
}

func TestSortCLientsByCreatedAtDesc(t *testing.T) {
	t0 := time.Now().Add(-1 * time.Minute)
	t1 := t0.Add(10 * time.Second)
	t2 := t1.Add(10 * time.Second)
	clients := []*oauth.Client{
		{CouchID: "a", Metadata: &metadata.CozyMetadata{CreatedAt: t2}},
		{CouchID: "d"},
		{CouchID: "c", Metadata: &metadata.CozyMetadata{CreatedAt: t0}},
		{CouchID: "e"},
		{CouchID: "b", Metadata: &metadata.CozyMetadata{CreatedAt: t1}},
	}
	oauth.SortClientsByCreatedAtDesc(clients)
	require.Len(t, clients, 5)
	assert.Equal(t, "a", clients[0].CouchID)
	assert.Equal(t, "b", clients[1].CouchID)
	assert.Equal(t, "c", clients[2].CouchID)
	assert.Equal(t, "d", clients[3].CouchID)
	assert.Equal(t, "e", clients[4].CouchID)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	setup := testutils.NewSetup(m, "oauth_client")
	testInstance = setup.GetTestInstance()
	os.Exit(m.Run())
	setup.Cleanup()
}
