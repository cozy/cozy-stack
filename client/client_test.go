package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/stretchr/testify/assert"
)

type testAssertReq func(*http.Request)
type testTransport struct {
	assertFn testAssertReq
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.assertFn(req)
	return nil, errors.New("ok")
}

func testClient(assertFn testAssertReq) *http.Client {
	return &http.Client{Transport: &testTransport{assertFn}}
}

func TestClientWithOAuth(t *testing.T) {
	c := &Client{
		Domain: "foobar",
		Scheme: "https",
		Client: testClient(func(req *http.Request) {
			header := req.Header
			assert.Equal(t, "", header.Get("Authorization"))
			assert.Equal(t, "application/json", header.Get("Content-Type"))
			assert.Equal(t, "application/json", header.Get("Accept"))
			assert.Equal(t, &url.URL{
				Scheme: "https",
				Host:   "foobar",
				Path:   "/auth/register",
			}, req.URL)
			var v auth.Client
			json.NewDecoder(req.Body).Decode(&v)
			assert.EqualValues(t, v, auth.Client{
				RedirectURIs: []string{"http://redirectto/"},
				ClientName:   "name",
				ClientKind:   "kind",
				ClientURI:    "uri",
				SoftwareID:   "github.com/cozy/cozy-stack",
			})
		}),
		AuthClient: &auth.Client{
			RedirectURIs: []string{"http://redirectto/"},
			ClientName:   "name",
			ClientKind:   "kind",
			ClientURI:    "uri",
		},
	}
	_, err := c.Req(&request.Options{
		Method: "PUT",
		Path:   "/p/a/t/h",
	})
	assert.Error(t, err)
}

func TestClientWithoutOAuth(t *testing.T) {
	type testjson struct {
		Key string `json:"key"`
	}
	c := &Client{
		Domain:     "foobar",
		Scheme:     "https",
		UserAgent:  "user/agent",
		Authorizer: &request.BearerAuthorizer{Token: "token"},
		Client: testClient(func(req *http.Request) {
			header := req.Header
			assert.Equal(t, "Bearer token", header.Get("Authorization"))
			assert.Equal(t, "user/agent", header.Get("User-Agent"))
			assert.Equal(t, "application/json", header.Get("Content-Type"))
			assert.Equal(t, "application/json", header.Get("Accept"))
			assert.Equal(t, &url.URL{
				Scheme:   "https",
				Host:     "foobar",
				Path:     "/p/a/t/h",
				RawQuery: "q=value",
			}, req.URL)
			var v testjson
			json.NewDecoder(req.Body).Decode(&v)
			assert.Equal(t, v.Key, "Value")
		}),
	}

	body, err := request.WriteJSON(&testjson{Key: "Value"})
	assert.NoError(t, err)

	_, err = c.Req(&request.Options{
		Method:  "PUT",
		Path:    "/p/a/t/h",
		Queries: url.Values{"q": {"value"}},
		Headers: request.Headers{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		Body: body,
	})
	assert.Error(t, err)
}
