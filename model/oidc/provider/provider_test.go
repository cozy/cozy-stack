package provider

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigRequireIssuerOrTokenURL(t *testing.T) {
	config.UseTestFile(t)
	config.GetConfig().Authentication = map[string]interface{}{
		"missing-issuer-and-token-url": map[string]interface{}{
			"oidc": map[string]interface{}{
				"client_id":        "provider-client-id",
				"id_token_jwk_url": "https://example.org/jwks",
			},
		},
	}

	_, err := LoadConfig("missing-issuer-and-token-url", RequireIssuerOrTokenURL)
	require.EqualError(t, err, "The issuer or token_url is missing for this context")
}

func TestVerifyLogoutTokenFailsWhenIssuerCannotBeResolved(t *testing.T) {
	config.UseTestFile(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kid := "provider-test-key"
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jwks" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		e := big.NewInt(int64(privateKey.PublicKey.E)).Bytes()
		payload := map[string]interface{}{
			"keys": []map[string]string{{
				"kty": "RSA",
				"use": "sig",
				"kid": kid,
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(e),
			}},
		}
		require.NoError(t, json.NewEncoder(w).Encode(payload))
	}))
	defer jwksServer.Close()

	conf := &Config{
		ClientID:      "provider-client-id",
		IDTokenKeyURL: jwksServer.URL + "/jwks",
		TokenURL:      "://bad-token-url",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":    "https://issuer.example/provider",
		"aud":    "provider-client-id",
		"iat":    time.Now().Unix(),
		"jti":    "logout-jti",
		"sid":    "provider-sid",
		"events": map[string]interface{}{backchannelLogoutEvent: map[string]interface{}{}},
	})
	token.Header["kid"] = kid
	raw, err := token.SignedString(privateKey)
	require.NoError(t, err)

	_, err = VerifyLogoutToken(raw, "provider-context", conf)
	require.ErrorContains(t, err, "cannot resolve issuer for OIDC context provider-context")
}
